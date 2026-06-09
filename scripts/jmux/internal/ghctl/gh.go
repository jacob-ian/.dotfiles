// Package ghctl wraps the GitHub CLI (`gh`). Each call takes the directory to
// run in, from which gh resolves the target repo.
package ghctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

// PR is the subset of gh PR fields the review flow needs.
type PR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	IsDraft     bool   `json:"isDraft"`
	HeadRefName string `json:"headRefName"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
}

// prFields are the JSON fields requested for both list and single-PR views.
const prFields = "number,title,author,isDraft,headRefName"

// Available reports whether the gh CLI is on PATH.
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// ListPRs returns the open pull requests for the repo at dir, newest first.
func ListPRs(dir string) ([]PR, error) {
	out, err := run(dir, "pr", "list", "--limit", "200", "--json", prFields)
	if err != nil {
		return nil, err
	}
	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, err
	}
	return prs, nil
}

// GetPR returns a single PR by number, for the direct `jmux pr <num>` path.
func GetPR(dir string, num int) (PR, error) {
	out, err := run(dir, "pr", "view", strconv.Itoa(num), "--json", prFields)
	if err != nil {
		return PR{}, err
	}
	var p PR
	if err := json.Unmarshal(out, &p); err != nil {
		return PR{}, err
	}
	return p, nil
}

// run executes `gh args...` in dir, returning stdout. On failure the error
// carries gh's stderr (e.g. an auth or network message).
func run(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return out, nil
}

// View returns `gh pr view <num> --comments`: the PR body plus comment threads.
// stderr is folded in so a failure still renders something in the preview.
func View(dir string, num int) string {
	cmd := exec.Command("gh", "pr", "view", strconv.Itoa(num), "--comments")
	cmd.Dir = dir
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// ViewRepo is View for a PR in another repo, qualified by owner/repo rather
// than a working directory — used by the cross-repo (`jmux pr`) preview.
func ViewRepo(nameWithOwner string, num int) string {
	cmd := exec.Command("gh", "pr", "view", strconv.Itoa(num), "--repo", nameWithOwner, "--comments")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// SearchResult is a cross-repo PR from `gh search prs`, which (unlike pr list)
// spans every repo you can see but does not expose the head branch.
type SearchResult struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	IsDraft    bool   `json:"isDraft"`
	Repository struct {
		Name          string `json:"name"`
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
}

const searchFields = "number,title,author,isDraft,repository"

// SearchAssignedPRs returns open PRs across all repos that either request your
// review or are assigned to you. GitHub search ANDs its qualifiers, so the two
// are run as separate queries and merged, deduped by repo+number.
func SearchAssignedPRs() ([]SearchResult, error) {
	seen := map[string]bool{}
	var out []SearchResult
	for _, qualifier := range []string{"--review-requested=@me", "--assignee=@me"} {
		o, err := run("", "search", "prs", "--state=open", "--limit", "100", qualifier, "--json", searchFields)
		if err != nil {
			return nil, err
		}
		var rs []SearchResult
		if err := json.Unmarshal(o, &rs); err != nil {
			return nil, err
		}
		for _, r := range rs {
			key := r.Repository.NameWithOwner + "#" + strconv.Itoa(r.Number)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, r)
		}
	}
	return out, nil
}

// HeadRef resolves a PR's head branch by owner/repo and number — the field
// `gh search prs` omits, fetched only for the PR actually selected.
func HeadRef(nameWithOwner string, num int) (string, error) {
	out, err := run("", "pr", "view", strconv.Itoa(num), "--repo", nameWithOwner, "--json", "headRefName", "-q", ".headRefName")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
