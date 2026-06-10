// Package ghctl talks to GitHub: data queries (list/search/lookup) via the
// go-gh SDK (in-process, reusing gh's auth); the PR preview via `gh pr view`.
package ghctl

import (
	"context"
	"fmt"
	"net/url"
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

// SearchResult is a cross-repo PR from search, which omits the head branch.
type SearchResult struct {
	Number     int
	Title      string
	IsDraft    bool
	Repository struct {
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

// searchResponse is the REST shape of `GET /search/issues`.
type searchResponse struct {
	Items []struct {
		Number        int    `json:"number"`
		Title         string `json:"title"`
		Draft         bool   `json:"draft"`
		RepositoryURL string `json:"repository_url"`
		User          struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"items"`
}

var (
	loginOnce sync.Once
	login     string
	loginErr  error
)

// currentLogin resolves the authenticated user's login, to expand "@me".
func currentLogin() (string, error) {
	loginOnce.Do(func() {
		var u struct {
			Login string `json:"login"`
		}
		if err := get("user", &u); err != nil {
			loginErr = err
			return
		}
		login = u.Login
	})
	return login, loginErr
}

// SearchAssignedPRs returns open PRs across all repos that request your review
// or are assigned to you, deduped by repo+number. Search ANDs qualifiers, so
// the two are run separately and merged.
func SearchAssignedPRs() ([]SearchResult, error) {
	me, err := currentLogin()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []SearchResult
	for _, qualifier := range []string{"review-requested:" + me, "assignee:" + me} {
		q := "is:pr is:open " + qualifier
		var resp searchResponse
		if err := get("search/issues?per_page=100&q="+url.QueryEscape(q), &resp); err != nil {
			return nil, err
		}
		for _, it := range resp.Items {
			slug := repoSlug(it.RepositoryURL)
			key := slug + "#" + strconv.Itoa(it.Number)
			if seen[key] {
				continue
			}
			seen[key] = true
			var r SearchResult
			r.Number, r.Title, r.IsDraft = it.Number, it.Title, it.Draft
			r.Author.Login = it.User.Login
			r.Repository.NameWithOwner = slug
			if i := strings.LastIndex(slug, "/"); i >= 0 {
				r.Repository.Name = slug[i+1:]
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// repoSlug extracts "owner/repo" from a search item's repository_url.
func repoSlug(repositoryURL string) string {
	const marker = "/repos/"
	if i := strings.Index(repositoryURL, marker); i >= 0 {
		return repositoryURL[i+len(marker):]
	}
	return repositoryURL
}

// ViewRepo renders `gh pr view --comments` for the preview, bounded by
// previewTimeout so a slow fetch is killed rather than piling up. stderr is
// folded in so a failure still shows something.
func ViewRepo(slug string, num int) string {
	ctx, cancel := context.WithTimeout(context.Background(), previewTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(num), "--repo", slug, "--comments")
	out, _ := cmd.CombinedOutput()
	return string(out)
}
