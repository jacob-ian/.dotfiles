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
