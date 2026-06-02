// Package workspace presents the mid-breadth overview of places you're actively
// working: every open tmux session unioned with every feature worktree. It sits
// between the all-dirs picker (everything on disk) and a bare worktree list, and
// manages the full lifecycle — add a worktree workspace (ctrl-t) or remove a
// workspace (ctrl-x), where removal deletes the worktree only when one backs it.
package workspace

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"jmux/internal/fzfutil"
	"jmux/internal/notify"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/tmuxctl"
	"jmux/internal/worktree"
)

// Workspaces returns the overview rows: feature worktrees first, then the
// directories of any open sessions that aren't already listed, deduped by
// resolved path.
func Workspaces() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = repo.TrimSlash(p)
		key := resolve(p)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, p)
	}
	for _, w := range worktree.AllFeatureWorktrees() {
		add(w)
	}
	for _, d := range openSessionDirs() {
		add(d)
	}
	slices.Sort(out)
	return out
}

// openSessionDirs returns the originating directory of every open tmux session,
// preferring the @jmux_dir stamp and falling back to the active pane's path for
// sessions created before stamping existed.
func openSessionDirs() []string {
	var out []string
	for _, s := range tmuxctl.ListSessions() {
		dir := tmuxctl.SessionOption(s, "@jmux_dir")
		if dir == "" {
			dir = tmuxctl.SessionPath(s)
		}
		if dir != "" {
			out = append(out, dir)
		}
	}
	return out
}

func resolve(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

// RunPicker handles `jmux workspace`. With --print it lists rows to stdout (the
// form the ctrl-x reload re-runs against); otherwise it opens the fzf overview.
func RunPicker(args []string) {
	fs := flag.NewFlagSet("workspace", flag.ExitOnError)
	printOnly := fs.Bool("print", false, "Print workspace paths and exit")
	fs.Parse(args)

	dirs := Workspaces()

	if *printOnly {
		fmt.Println(strings.Join(dirs, "\n"))
		return
	}

	if len(dirs) == 0 {
		notify.Info("No workspaces found")
		return
	}

	self, err := os.Executable()
	if err != nil {
		self = "jmux"
	}

	addBind := fmt.Sprintf("ctrl-t:become(%s workspace add)", self)
	removeBind := fmt.Sprintf(
		"ctrl-x:execute-silent(%s workspace remove --path {} --quiet)+reload(%s workspace --print)",
		self, self,
	)
	togglePreview := "ctrl-/:toggle-preview"

	sel, err := fzfutil.Pick(dirs, fzfutil.Options{
		Prompt:        "workspace> ",
		Header:        "ctrl-t: add worktree · ctrl-x: remove workspace · ctrl-/: toggle preview",
		Bindings:      []string{addBind, removeBind, togglePreview},
		Preview:       fmt.Sprintf("%s workspace preview --path {}", self),
		PreviewWindow: "follow",
	})
	if err != nil || sel == "" {
		return
	}
	if err := session.Open(repo.TrimSlash(sel), session.OpenOptions{}); err != nil {
		notify.Error(err.Error())
	}
}

// RunAdd handles `jmux workspace add`: pick a bare repo, then run the worktree
// branch flow against it.
func RunAdd() {
	repos := repo.BareRepos()
	if len(repos) == 0 {
		notify.Info("No bare repos found")
		return
	}
	sel, err := fzfutil.Pick(repos, fzfutil.Options{Prompt: "repo> "})
	if err != nil || sel == "" {
		return
	}
	worktree.AddWorktree(repo.TrimSlash(sel))
}

// RunRemove handles `jmux workspace remove --path P [--quiet]`. A worktree-backed
// workspace is removed (worktree + session); any other workspace just has its
// session closed — the directory is never deleted.
func RunRemove(args []string) {
	fs := flag.NewFlagSet("workspace remove", flag.ExitOnError)
	pathArg := fs.String("path", "", "Workspace path to remove")
	quiet := fs.Bool("quiet", false, "Suppress tmux display-message status")
	fs.Parse(args)

	path := repo.TrimSlash(*pathArg)
	if path == "" {
		return
	}

	if worktree.IsManagedWorktree(path) {
		worktree.Remove(path, *quiet)
		return
	}

	name := session.Name(path)
	if !tmuxctl.HasSession(name) {
		if !*quiet {
			notify.Infof("No session for '%s'", filepath.Base(path))
		}
		return
	}
	session.Kill(name)
	if !*quiet {
		notify.Infof("Closed workspace '%s'", filepath.Base(path))
	}
}
