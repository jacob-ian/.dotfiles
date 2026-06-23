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
	Repository  struct {
		Name          string
		NameWithOwner string
	}
	Author struct {
		Login string
	}
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
	User struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (p pull) toPR() PR {
	pr := PR{Number: p.Number, Title: p.Title, IsDraft: p.Draft, HeadRefName: p.Head.Ref}
	pr.Author.Login = p.User.Login
	return pr
}

// ListPRs returns the open PRs for the owner/repo slug, newest first.
func ListPRs(slug string) ([]PR, error) {
	var pulls []pull
	if err := get(fmt.Sprintf("repos/%s/pulls?state=open&per_page=100", slug), &pulls); err != nil {
		return nil, err
	}
	prs := make([]PR, len(pulls))
	for i, p := range pulls {
		prs[i] = p.toPR()
	}
	return prs, nil
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
}
fragment prFields on PullRequest {
  number
  title
  isDraft
  headRefName
  author { login }
  repository { name nameWithOwner }
}`

// searchMyPrsQueryResponse holds one aliased search block per review-queue qualifier.
type searchMyPrsQueryResponse struct {
	ReviewRequested prSearchBlock `json:"reviewRequested"`
	Assigned        prSearchBlock `json:"assigned"`
	Authored        prSearchBlock `json:"authored"`
}

type prSearchBlock struct {
	Nodes []struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		IsDraft     bool   `json:"isDraft"`
		HeadRefName string `json:"headRefName"`
		Author      struct {
			Login string `json:"login"`
		} `json:"author"`
		Repository struct {
			Name          string `json:"name"`
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
	} `json:"nodes"`
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
	cl, err := graphqlClient()
	if err != nil {
		return nil, fmt.Errorf("github client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	scope := orgQualifier(orgs)
	variables := map[string]any{
		"reviewRequested": "is:pr is:open review-requested:@me" + scope,
		"assigned":        "is:pr is:open assignee:@me" + scope,
		"authored":        "is:pr is:open author:@me" + scope,
	}
	var resp searchMyPrsQueryResponse
	if err := cl.DoWithContext(ctx, searchMyPRsQuery, variables, &resp); err != nil {
		return nil, fmt.Errorf("search PRs: %w", err)
	}

	seen := map[string]bool{}
	var out []SearchResult
	for _, block := range []prSearchBlock{resp.ReviewRequested, resp.Assigned, resp.Authored} {
		for _, n := range block.Nodes {
			if n.Number == 0 {
				continue // non-PR union member or null node
			}
			key := n.Repository.NameWithOwner + "#" + strconv.Itoa(n.Number)
			if seen[key] {
				continue
			}
			seen[key] = true
			var r SearchResult
			r.Number, r.Title, r.IsDraft, r.HeadRefName = n.Number, n.Title, n.IsDraft, n.HeadRefName
			r.Author.Login = n.Author.Login
			r.Repository.Name = n.Repository.Name
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

// ViewRepo renders `gh pr view --comments` for the preview
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
