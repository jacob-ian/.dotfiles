// Package pr checks a GitHub pull request out into a worktree and opens a
// review session.
package pr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"jmux/internal/fzfutil"
	"jmux/internal/ghctl"
	"jmux/internal/gitctl"
	"jmux/internal/notify"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/spinner"
	"jmux/internal/tag"
	"jmux/internal/tmuxctl"
	"jmux/internal/worktree"
)

const tagKind = "pr"

const agentLogin = "g3rtrude"

type tagData struct {
	Number int `json:"number"`
}

var registerOnce sync.Once

// Register wires this package's workspace-tag renderer. \uf407 is nf-oct-git_pull_request.
func Register() {
	registerOnce.Do(func() {
		tag.Register(tagKind, func(data json.RawMessage) (string, tag.Color) {
			var d tagData
			if json.Unmarshal(data, &d) != nil || d.Number == 0 {
				return "", ""
			}
			return fmt.Sprintf("\uf407 #%d", d.Number), tag.Cyan
		})
	})
}

var errNoGH = errors.New("gh CLI not found — install the GitHub CLI to review PRs")

// errNoBareRepos is shown as plain info rather than a red error.
var errNoBareRepos = errors.New("no bare repos found")

// RunRepo handles `jmux pr <dir>`: pick one of the repo's open PRs and review it.
func RunRepo(cfg repo.Config, dir string) error {
	if !ghctl.Available() {
		return errNoGH
	}
	bareRoot, slug, err := resolveRepoSlug(cfg, dir)
	if errors.Is(err, errNoBareRepos) {
		notify.Info("No bare repos found")
		return nil
	}
	if err != nil {
		return err
	}
	if bareRoot == "" {
		return nil
	}

	itemsCmd := fmt.Sprintf("%s fzf pr items --repo %s", tmuxctl.ShellQuote(fzfutil.Self()), tmuxctl.ShellQuote(slug))
	sel, err := pickPR("pr> ", itemsCmd, itemsCmd)
	if err != nil || sel == "" {
		return nil
	}

	_, num, ok := parseRepoNumber(sel)
	if !ok {
		return nil
	}
	headRef, err := resolveHeadRef(sel, slug, num)
	if err != nil {
		return err
	}
	return review(bareRoot, ghctl.PR{Number: num, HeadRefName: headRef, BaseRefName: rowBaseRef(sel)})
}

// RunNumber handles `jmux pr <num>`: review the PR directly, skipping the picker.
func RunNumber(cfg repo.Config, num int) error {
	if !ghctl.Available() {
		return errNoGH
	}
	bareRoot, slug, err := resolveRepoSlug(cfg, "")
	if errors.Is(err, errNoBareRepos) {
		notify.Info("No bare repos found")
		return nil
	}
	if err != nil {
		return err
	}
	if bareRoot == "" {
		return nil
	}
	p, err := ghctl.GetPR(slug, num)
	if err != nil {
		return fmt.Errorf("look up PR #%d: %s", num, gitctl.CleanErr(err))
	}
	return review(bareRoot, p)
}

// pickPR opens the PR picker; start:reload fills the list behind fzf's spinner
// so opening doesn't block on gh.
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
		Preview:       fmt.Sprintf("%s fzf pr preview {}", tmuxctl.ShellQuote(fzfutil.Self())),
		PreviewWindow: "right:60%:wrap",
		Delimiter:     "\t",
		WithNth:       "1",
	})
}

// resolveHeadRef reads the row's hidden head-branch field, falling back to gh
// for cached rows that predate it.
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

// prEditorCmd opens nvim on the PR conversation view; VimEnter runs after the
// lazy plugin is loaded.
const prEditorCmd = `nvim -c "autocmd VimEnter * ++once lua require('jmux').pr.view()"`

// review checks the PR out into a worktree and opens its session.
func review(bareRoot string, p ghctl.PR) error {
	var path, notice string
	err := spinner.Run(fmt.Sprintf("opening PR #%d…", p.Number), func(phase chan<- string) (err error) {
		path, notice, err = checkoutWorktree(bareRoot, p, phase)
		return
	})
	if err != nil {
		return fmt.Errorf("checkout PR #%d: %s", p.Number, gitctl.CleanErr(err))
	}
	tag.Set(path, tagKind, tag.New(tagKind, "", tagData{Number: p.Number}))
	if err := session.Open(path, session.OpenOptions{
		WithClaude: true,
		InstallCmd: worktree.DetectInstallCmd(path),
		EditorCmd:  prEditorCmd,
	}); err != nil {
		return err
	}
	if notice != "" {
		notify.Info(notice)
	}
	return nil
}

// checkoutWorktree returns the PR's head-branch worktree, created if absent
// and synced to origin otherwise; notice is a sync outcome worth showing.
func checkoutWorktree(bareRoot string, p ghctl.PR, phase chan<- string) (path, notice string, err error) {
	branch := p.HeadRefName

	// The base is fetched even when the worktree is reused so the diff's
	// base...HEAD is current; a failed fetch (offline) must not stop the open.
	base := p.BaseRefName
	if base == "" {
		base = gitctl.DefaultBranch(bareRoot)
	}
	if base != "" {
		phase <- "fetching " + base + "…"
		_ = gitctl.FetchBranch(bareRoot, base)
	}

	path = filepath.Join(bareRoot, branch)
	if !repo.IsDir(path) {
		path = gitctl.WorktreeForBranch(bareRoot, branch)
	}
	if path != "" && repo.IsDir(path) {
		phase <- "syncing " + branch + "…"
		// Offline, the open goes ahead on the stale head like the base fetch does.
		res, err := Sync(bareRoot, path, branch)
		switch {
		case err != nil:
			notice = "sync " + branch + ": " + gitctl.CleanErr(err)
		case res.Outcome == Reset, res.Outcome == Diverged:
			notice = res.String()
		}
		return path, notice, nil
	}

	path = filepath.Join(bareRoot, branch)
	phase <- "fetching " + branch + "…"
	if err := gitctl.FetchBranch(bareRoot, branch); err != nil {
		return "", "", err
	}
	phase <- "creating worktree…"
	if err := gitctl.WorktreeAdd(bareRoot, path, branch, "", false); err != nil {
		return "", "", err
	}
	phase <- "copying env files…"
	worktree.CopyEnvFiles(bareRoot, path)
	return path, "", nil
}

// resolveRepoSlug returns the bare repo containing dir (cwd when "", picked when
// none) and its "owner/repo" slug; a cancelled pick is empty with a nil error.
func resolveRepoSlug(cfg repo.Config, dir string) (bareRoot, slug string, err error) {
	bareRoot, err = resolveBareRoot(cfg, dir)
	if err != nil || bareRoot == "" {
		return "", "", err
	}
	if slug = gitctl.RepoSlug(bareRoot); slug == "" {
		return "", "", errors.New("could not resolve the repo's origin remote")
	}
	return bareRoot, slug, nil
}

func resolveBareRoot(cfg repo.Config, start string) (string, error) {
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("reading cwd: %w", err)
		}
		start = cwd
	}
	if bareRoot := gitctl.CommonDir(start); bareRoot != "" {
		return bareRoot, nil
	}

	repos := cfg.BareRepos()
	if len(repos) == 0 {
		return "", errNoBareRepos
	}
	sel, err := fzfutil.Pick(repos, fzfutil.Options{Prompt: "repo> "})
	if err != nil || sel == "" {
		return "", nil
	}
	return repo.TrimSlash(sel), nil
}

// formatRow renders "<marker>owner/repo#12  [draft] Title  ·  author".
func formatRow(marker, slug string, num int, draft bool, title, login string) string {
	state := ""
	if draft {
		state = "[draft] "
	}
	return fmt.Sprintf("%s%s#%d  %s%s  ·  %s", marker, slug, num, state, title, login)
}

func rowMarker(author string) string {
	if author == agentLogin {
		return repo.AgentMarker
	}
	return "  "
}

func byline(author, owner string) string {
	if owner == "" {
		return author
	}
	return owner + " via " + author
}

// formatItemsRow appends the head and base branches as hidden tab fields so
// selection can read them back without a gh lookup.
func formatItemsRow(marker, slug string, num int, draft bool, title, login, headRef, baseRef string) string {
	return formatRow(marker, slug, num, draft, title, login) + "\t" + headRef + "\t" + baseRef
}

// rowField returns the n-th tab-separated field, or "" for older cached rows
// that carry fewer fields.
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
