// Package pr checks a GitHub pull request out into a worktree and opens a
// review session. A PR is just a branch, so it reuses the worktree + session
// machinery of `jmux worktree add`, sourcing PRs from `gh`.
package pr

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"jmux/internal/fzfutil"
	"jmux/internal/ghctl"
	"jmux/internal/gitctl"
	"jmux/internal/notify"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/worktree"
)

// RunRepo handles `jmux pr <dir>`: list one repo's open PRs and review the
// choice. dir picks the repo ("" or "." = current); the global queue is
// RunAssigned.
func RunRepo(dir string) {
	if !ghctl.Available() {
		notify.Error("gh CLI not found — install the GitHub CLI to review PRs")
		return
	}

	bareRoot, ok := resolveBareRoot(dir)
	if !ok {
		return
	}

	slug := gitctl.RepoSlug(bareRoot)
	if slug == "" {
		notify.Error("Could not resolve the repo's origin remote")
		return
	}
	prs, err := ghctl.ListPRs(slug)
	if err != nil {
		notify.Errorf("list PRs: %s", gitctl.CleanErr(err))
		return
	}
	if len(prs) == 0 {
		notify.Info("No open pull requests")
		return
	}

	items := make([]string, len(prs))
	byNum := make(map[int]ghctl.PR, len(prs))
	for i, p := range prs {
		items[i] = formatRow(slug, p.Number, p.IsDraft, p.Title, p.Author.Login)
		byNum[p.Number] = p
	}

	self, err := os.Executable()
	if err != nil {
		self = "jmux"
	}
	previewCmd := fmt.Sprintf("%s pr preview {}", shellQuote(self))

	sel, err := fzfutil.Pick(items, fzfutil.Options{
		Prompt:        "pr> ",
		Header:        "enter: review · ctrl-/: toggle preview",
		Bindings:      []string{"ctrl-/:toggle-preview"},
		Preview:       previewCmd,
		PreviewWindow: "right:60%:wrap",
	})
	if err != nil || sel == "" {
		return
	}

	_, num, ok := parseRepoNumber(sel)
	if !ok {
		return
	}
	if p, ok := byNum[num]; ok {
		Review(bareRoot, p)
	}
}

// RunNumber handles `jmux pr <num>`: review the PR directly, skipping the picker.
func RunNumber(num int) {
	if !ghctl.Available() {
		notify.Error("gh CLI not found — install the GitHub CLI to review PRs")
		return
	}
	bareRoot, ok := resolveBareRoot("")
	if !ok {
		return
	}
	slug := gitctl.RepoSlug(bareRoot)
	if slug == "" {
		notify.Error("Could not resolve the repo's origin remote")
		return
	}
	p, err := ghctl.GetPR(slug, num)
	if err != nil {
		notify.Errorf("look up PR #%d: %s", num, gitctl.CleanErr(err))
		return
	}
	Review(bareRoot, p)
}

// Review checks the PR out into a worktree and opens its session: nvim, a
// paired claude window, and the install window.
func Review(bareRoot string, p ghctl.PR) {
	path, err := checkoutWorktree(bareRoot, p)
	if err != nil {
		notify.Errorf("checkout PR #%d: %s", p.Number, gitctl.CleanErr(err))
		return
	}
	if err := session.Open(path, session.OpenOptions{
		WithClaude: true,
		InstallCmd: worktree.DetectInstallCmd(path),
	}); err != nil {
		notify.Error(err.Error())
	}
}

// checkoutWorktree returns the PR's head-branch worktree, fetching and creating
// it (tracking origin/<branch>) if absent or reusing an existing one. Env files
// are copied across as for any feature worktree.
func checkoutWorktree(bareRoot string, p ghctl.PR) (string, error) {
	branch := p.HeadRefName
	path := filepath.Join(bareRoot, branch)
	if repo.IsDir(path) {
		return path, nil
	}
	if existing := gitctl.WorktreeForBranch(bareRoot, branch); existing != "" && repo.IsDir(existing) {
		return existing, nil
	}
	if err := gitctl.FetchBranch(bareRoot, branch); err != nil {
		return "", err
	}
	if err := gitctl.WorktreeAdd(bareRoot, path, branch, false); err != nil {
		return "", err
	}
	worktree.CopyEnvFiles(bareRoot, path)
	return path, nil
}

// resolveBareRoot returns the bare repo containing start (cwd when start is ""),
// or one the user picks.
func resolveBareRoot(start string) (string, bool) {
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			notify.Error("Failed to read cwd")
			return "", false
		}
		start = cwd
	}
	if bareRoot := gitctl.CommonDir(start); bareRoot != "" {
		return bareRoot, true
	}

	repos := repo.BareRepos()
	if len(repos) == 0 {
		notify.Info("No bare repos found")
		return "", false
	}
	sel, err := fzfutil.Pick(repos, fzfutil.Options{Prompt: "repo> "})
	if err != nil || sel == "" {
		return "", false
	}
	return repo.TrimSlash(sel), true
}

// formatRow renders a picker row: "owner/repo#12  [draft] Title  ·  author".
func formatRow(slug string, num int, draft bool, title, login string) string {
	tag := ""
	if draft {
		tag = "[draft] "
	}
	return fmt.Sprintf("%s#%d  %s%s  ·  %s", slug, num, tag, title, login)
}

// ParseNumber extracts the leading PR number from a picker row or CLI argument.
func ParseNumber(s string) (int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil {
		return 0, false
	}
	return n, true
}

// shellQuote single-quotes s for the fzf preview command, which runs via sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
