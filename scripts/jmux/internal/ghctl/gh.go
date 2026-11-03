// Package ghctl talks to GitHub: data queries (list/search/lookup) via the
// go-gh SDK (in-process, reusing gh's auth); the PR preview via `gh pr view`.
package ghctl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

const (
	previewTimeout = 5 * time.Second  // per-keystroke `gh pr view` preview
	queryTimeout   = 15 * time.Second // SDK data calls
)

// PR is the subset of fields the review flow needs.
type PR struct {
	Number      int
	Title       string
	IsDraft     bool
	HeadRefName string
	BaseRefName string
	Author      struct {
		Login string
	}
}

// SearchResult is a cross-repo PR from search.
type SearchResult struct {
	Number      int
	Title       string
	IsDraft     bool
	HeadRefName string
	BaseRefName string
	Repository  struct {
		NameWithOwner string
	}
	Author struct {
		Login string
	}
	// Owner is the human driving a bot-authored PR (whoever last requested
	// review or marked it ready, else its assignee); "" for human authors.
	Owner string
}

// Available reports whether the gh CLI is on PATH (needed for the preview).
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

var (
	clientOnce sync.Once
	client     *api.RESTClient
	clientErr  error
)

// rest returns a cached REST client authed via gh's resolved token.
func rest() (*api.RESTClient, error) {
	clientOnce.Do(func() { client, clientErr = api.DefaultRESTClient() })
	return client, clientErr
}

// get performs a bounded GET, decoding the JSON response into out.
func get(path string, out any) error {
	cl, err := rest()
	if err != nil {
		return fmt.Errorf("github client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	if err := cl.DoWithContext(ctx, "GET", path, nil, out); err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	return nil
}

// pull is the REST shape of a pull request (the fields we read).
type pull struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Draft  bool   `json:"draft"`
	Head   struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (p pull) toPR() PR {
	pr := PR{Number: p.Number, Title: p.Title, IsDraft: p.Draft, HeadRefName: p.Head.Ref, BaseRefName: p.Base.Ref}
	pr.Author.Login = p.User.Login
	return pr
}

// GetPR returns a single PR by number (the `jmux pr <num>` path).
func GetPR(slug string, num int) (PR, error) {
	var p pull
	if err := get(fmt.Sprintf("repos/%s/pulls/%d", slug, num), &p); err != nil {
		return PR{}, err
	}
	return p.toPR(), nil
}

// HeadRef resolves a PR's head branch — the field search omits.
func HeadRef(slug string, num int) (string, error) {
	p, err := GetPR(slug, num)
	if err != nil {
		return "", err
	}
	return p.HeadRefName, nil
}

// searchMyPRsQuery aliases the three review-queue searches into one round-trip;
// `@me` resolves server-side, so no separate login lookup is needed.
const searchMyPRsQuery = `
query($reviewRequested: String!, $assigned: String!, $authored: String!) {
  reviewRequested: search(query: $reviewRequested, type: ISSUE, first: 100) { nodes { ...prFields } }
  assigned:        search(query: $assigned,        type: ISSUE, first: 100) { nodes { ...prFields } }
  authored:        search(query: $authored,        type: ISSUE, first: 100) { nodes { ...prFields } }
}` + prFieldsFragment

// searchRepoQuery is the single-search form for one repo's open PRs.
const searchRepoQuery = `
query($prs: String!) {
  prs: search(query: $prs, type: ISSUE, first: 100) { nodes { ...prFields } }
}` + prFieldsFragment

// prFieldsFragment is the PR selection every picker row is built from, so
// the cross-repo queue and a single repo's list render the same way.
const prFieldsFragment = `
fragment prFields on PullRequest {
  number
  title
  isDraft
  headRefName
  baseRefName
  author { login __typename }
  repository { nameWithOwner }
  assignees(first: 1) { nodes { login } }
  timelineItems(last: 5, itemTypes: [REVIEW_REQUESTED_EVENT, READY_FOR_REVIEW_EVENT]) {
    nodes {
      ... on ReviewRequestedEvent { actor { login __typename } }
      ... on ReadyForReviewEvent { actor { login __typename } }
    }
  }
}`

type prSearchBlock struct {
	Nodes []prSearchNode `json:"nodes"`
}

type prSearchNode struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	IsDraft     bool   `json:"isDraft"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	Author      actor  `json:"author"`
	Repository  struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Assignees struct {
		Nodes []struct {
			Login string `json:"login"`
		} `json:"nodes"`
	} `json:"assignees"`
	TimelineItems struct {
		Nodes []struct {
			Actor actor `json:"actor"`
		} `json:"nodes"`
	} `json:"timelineItems"`
}

type actor struct {
	Login    string `json:"login"`
	Typename string `json:"__typename"`
}

// owner is the human behind a bot-authored PR: the newest non-bot actor to
// request review or mark it ready (CODEOWNERS requests come from the bot
// itself, so those are skipped), else the first assignee. "" for a human
// author, or a bot PR nobody has picked up.
func (n prSearchNode) owner() string {
	if n.Author.Typename != "Bot" {
		return ""
	}
	for i := len(n.TimelineItems.Nodes) - 1; i >= 0; i-- {
		if a := n.TimelineItems.Nodes[i].Actor; a.Login != "" && a.Typename != "Bot" {
			return a.Login
		}
	}
	if len(n.Assignees.Nodes) > 0 {
		return n.Assignees.Nodes[0].Login
	}
	return ""
}

var (
	gqlOnce sync.Once
	gql     *api.GraphQLClient
	gqlErr  error
)

// graphqlClient returns a cached GraphQL client authed via gh's resolved token.
func graphqlClient() (*api.GraphQLClient, error) {
	gqlOnce.Do(func() { gql, gqlErr = api.DefaultGraphQLClient() })
	return gql, gqlErr
}

// SearchMyPRs returns open PRs across all repos that request your review, are
// assigned to you, or you authored, deduped by repo+number in that priority
// order. A non-empty orgs scopes the search to those owners.
func SearchMyPRs(orgs []string) ([]SearchResult, error) {
	scope := orgQualifier(orgs)
	return searchPRs(searchMyPRsQuery, map[string]string{
		"reviewRequested": "is:pr is:open review-requested:@me" + scope,
		"assigned":        "is:pr is:open assignee:@me" + scope,
		"authored":        "is:pr is:open author:@me" + scope,
	}, "reviewRequested", "assigned", "authored")
}

// ListPRs returns the open PRs for the owner/repo slug, newest first.
func ListPRs(slug string) ([]SearchResult, error) {
	return searchPRs(searchRepoQuery, map[string]string{
		"prs": "is:pr is:open sort:created-desc repo:" + slug,
	}, "prs")
}

// searchPRs runs one of the aliased search queries, each alias bound to its
// search string, and flattens the blocks in the given alias order, dropping
// PRs already seen under an earlier alias.
func searchPRs(query string, searches map[string]string, order ...string) ([]SearchResult, error) {
	cl, err := graphqlClient()
	if err != nil {
		return nil, fmt.Errorf("github client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	variables := make(map[string]any, len(searches))
	for alias, q := range searches {
		variables[alias] = q
	}
	resp := map[string]prSearchBlock{}
	if err := cl.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("search PRs: %w", err)
	}

	seen := map[string]bool{}
	var out []SearchResult
	for _, alias := range order {
		for _, n := range resp[alias].Nodes {
			if n.Number == 0 {
				continue // non-PR union member or null node
			}
			key := n.Repository.NameWithOwner + "#" + strconv.Itoa(n.Number)
			if seen[key] {
				continue
			}
			seen[key] = true
			var r SearchResult
			r.Number, r.Title, r.IsDraft, r.HeadRefName, r.BaseRefName = n.Number, n.Title, n.IsDraft, n.HeadRefName, n.BaseRefName
			r.Author.Login = n.Author.Login
			r.Owner = n.owner()
			r.Repository.NameWithOwner = n.Repository.NameWithOwner
			out = append(out, r)
		}
	}
	return out, nil
}

// orgQualifier builds the ` org:a org:b` search suffix. GitHub ORs repeated
// `org:` qualifiers, so any listed org matches; "" for no orgs.
func orgQualifier(orgs []string) string {
	var b strings.Builder
	for _, o := range orgs {
		b.WriteString(" org:")
		b.WriteString(o)
	}
	return b.String()
}

// ViewRepo renders `gh pr view --comments` for the preview.
func ViewRepo(slug string, num int) string {
	ctx, cancel := context.WithTimeout(context.Background(), previewTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(num), "--repo", slug, "--comments")
	width := os.Getenv("FZF_PREVIEW_COLUMNS")
	if width == "" {
		width = "80"
	}
	cmd.Env = append(os.Environ(), "GH_FORCE_TTY="+width)
	out, _ := cmd.CombinedOutput()
	return string(out)
}
