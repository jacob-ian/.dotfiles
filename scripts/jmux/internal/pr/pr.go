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
	"jmux/internal/spinner"
	"jmux/internal/tag"
	"jmux/internal/worktree"
)

// RunRepo handles `jmux pr <dir>`: list one repo's open PRs and review the
// choice. dir picks the repo ("" or "." = current); the global queue is
// RunAssigned.
func RunRepo(dir string) {
	if !ensureGH() {
		return
	}
	bareRoot, slug, ok := resolveRepoSlug(dir)
	if !ok {
		return
	}

	itemsCmd := fmt.Sprintf("%s pr items --repo %s", shellQuote(fzfutil.Self()), shellQuote(slug))
	sel, err := pickPR("pr> ", itemsCmd, itemsCmd)
	if err != nil || sel == "" {
		return
	}

	_, num, ok := parseRepoNumber(sel)
	if !ok {
		return
	}
	headRef, err := resolveHeadRef(sel, slug, num)
	if err != nil {
		notify.Error(err.Error())
		return
	}
	if err := review(bareRoot, ghctl.PR{Number: num, HeadRefName: headRef, BaseRefName: rowBaseRef(sel)}); err != nil {
		notify.Error(err.Error())
	}
}

// RunNumber handles `jmux pr <num>`: review the PR directly, skipping the picker.
func RunNumber(num int) {
	if !ensureGH() {
		return
	}
	bareRoot, slug, ok := resolveRepoSlug("")
	if !ok {
		return
	}
	p, err := ghctl.GetPR(slug, num)
	if err != nil {
		notify.Errorf("look up PR #%d: %s", num, gitctl.CleanErr(err))
		return
	}
	if err := review(bareRoot, p); err != nil {
		notify.Error(err.Error())
	}
}

// ensureGH reports whether the gh CLI is available, notifying when it isn't.
func ensureGH() bool {
	if ghctl.Available() {
		return true
	}
	notify.Error("gh CLI not found — install the GitHub CLI to review PRs")
	return false
}

// pickPR opens the PR picker: start:reload fills the list asynchronously behind
// fzf's spinner so the open doesn't block on gh, and the load event swaps the
// "loading…" prompt back. ctrl-r reloads via refreshCmd.
func pickPR(prompt, itemsCmd, refreshCmd string) (string, error) {
	return fzfutil.Pick(nil, fzfutil.Options{
		Prompt: "loading… ",
		Header: "enter: review · ctrl-r: refresh · ctrl-/: toggle preview",
		Bindings: []string{
			"ctrl-/:toggle-preview",
			fmt.Sprintf("start:reload(%s)", itemsCmd),
			fmt.Sprintf("load:change-prompt(%s)", prompt),
			fmt.Sprintf("ctrl-r:change-prompt(refreshing… )+reload(%s)", refreshCmd),
		},
		Preview:       fmt.Sprintf("%s pr preview {}", shellQuote(fzfutil.Self())),
		PreviewWindow: "right:60%:wrap",
		Delimiter:     "\t",
		WithNth:       "1",
	})
}

// resolveHeadRef returns a picker row's hidden head-branch field, falling back
// to a gh lookup for rows that predate the field.
func resolveHeadRef(sel, slug string, num int) (string, error) {
	if ref := rowHeadRef(sel); ref != "" {
		return ref, nil
	}
	ref, err := ghctl.HeadRef(slug, num)
	if err != nil || ref == "" {
		return "", fmt.Errorf("resolve branch for %s#%d: %s", slug, num, gitctl.CleanErr(err))
	}
	return ref, nil
}

// prEditorCmd opens nvim with the PR diff (`pd`) shown: a once-only VimEnter
// hook fires after startup, so the lazy plugin is loaded by the time it runs.
const prEditorCmd = `nvim -c "autocmd VimEnter * ++once lua require('jmux').pr.diff()"`

// review checks the PR out into a worktree and opens its session (nvim with the
// diff, a paired claude window, the install window), showing setup progress in
// a spinner.
func review(bareRoot string, p ghctl.PR) error {
	var path string
	err := spinner.Run(fmt.Sprintf("opening PR #%d…", p.Number), func(phase chan<- string) (err error) {
		path, err = checkoutWorktree(bareRoot, p, phase)
		return
	})
	if err != nil {
		return fmt.Errorf("checkout PR #%d: %s", p.Number, gitctl.CleanErr(err))
	}
	tag.Set(path, "pr", tag.Badge{Text: fmt.Sprintf("PR #%d", p.Number), Color: tag.Cyan})
	return session.Open(path, session.OpenOptions{
		WithClaude: true,
		InstallCmd: worktree.DetectInstallCmd(path),
		EditorCmd:  prEditorCmd,
	})
}

// checkoutWorktree returns the PR's head-branch worktree, fetching and creating
// it (tracking origin/<branch>) if absent or reusing an existing one. Env files
// are copied across as for any feature worktree.
func checkoutWorktree(bareRoot string, p ghctl.PR, phase chan<- string) (string, error) {
	branch := p.HeadRefName

	// Refresh the PR's base branch (even on the reuse paths below) so the diff's
	// base...HEAD resolves against an up-to-date base. baseRefName is the PR's
	// actual target, which isn't always the repo default (stacked or
	// release-branch PRs); fall back to the default branch when we don't know it.
	// Best effort: a failed fetch (e.g. offline) shouldn't stop the PR opening.
	base := p.BaseRefName
	if base == "" {
		base = gitctl.DefaultBranch(bareRoot)
	}
	if base != "" {
		phase <- "fetching " + base + "…"
		_ = gitctl.FetchBranch(bareRoot, base)
	}

	path := filepath.Join(bareRoot, branch)
	if repo.IsDir(path) {
		return path, nil
	}
	if existing := gitctl.WorktreeForBranch(bareRoot, branch); existing != "" && repo.IsDir(existing) {
		return existing, nil
	}
	phase <- "fetching " + branch + "…"
	if err := gitctl.FetchBranch(bareRoot, branch); err != nil {
		return "", err
	}
	phase <- "creating worktree…"
	if err := gitctl.WorktreeAdd(bareRoot, path, branch, "", false); err != nil {
		return "", err
	}
	phase <- "copying env files…"
	worktree.CopyEnvFiles(bareRoot, path)
	return path, nil
}

// resolveRepoSlug resolves the bare repo containing dir (cwd when "") — or one
// the user picks — plus its origin "owner/repo" slug.
func resolveRepoSlug(dir string) (bareRoot, slug string, ok bool) {
	bareRoot, ok = resolveBareRoot(dir)
	if !ok {
		return "", "", false
	}
	if slug = gitctl.RepoSlug(bareRoot); slug == "" {
		notify.Error("Could not resolve the repo's origin remote")
		return "", "", false
	}
	return bareRoot, slug, true
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
	marker := ""
	if draft {
		marker = "[draft] "
	}
	return fmt.Sprintf("%s#%d  %s%s  ·  %s", slug, num, marker, title, login)
}

// formatItemsRow appends the head and base branches as hidden tab fields (fzf
// shows only column 1) so selection can read them back without a gh lookup.
func formatItemsRow(slug string, num int, draft bool, title, login, headRef, baseRef string) string {
	return formatRow(slug, num, draft, title, login) + "\t" + headRef + "\t" + baseRef
}

// rowField returns the n-th tab-separated field of a picker row (0 = visible
// column), or "" when absent — tolerating older rows that carry fewer fields.
func rowField(line string, n int) string {
	parts := strings.Split(line, "\t")
	if n < len(parts) {
		return strings.TrimSpace(parts[n])
	}
	return ""
}

func rowHeadRef(line string) string { return rowField(line, 1) }
func rowBaseRef(line string) string { return rowField(line, 2) }

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
