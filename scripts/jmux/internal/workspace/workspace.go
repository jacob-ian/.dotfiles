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
	"jmux/internal/tag"
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

// displayRows renders each path as an fzf row "<display>\t<path>": the path
// followed by its colour-rendered badges. Bindings act on the hidden path field
// (column 2), so badges never leak into a path.
func displayRows(dirs []string) []string {
	tagged := tag.All()
	rows := make([]string, len(dirs))
	for i, d := range dirs {
		display := d
		if badges := tagged[resolve(d)]; len(badges) > 0 {
			parts := make([]string, len(badges))
			for j, b := range badges {
				parts[j] = b.Render()
			}
			display = d + "  ·  " + strings.Join(parts, "  ·  ")
		}
		rows[i] = display + "\t" + d
	}
	return rows
}

// rowPath returns the hidden path field of a workspace row (the text after the
// tab), falling back to the whole line for rows without one.
func rowPath(line string) string {
	if i := strings.LastIndexByte(line, '\t'); i >= 0 {
		return line[i+1:]
	}
	return line
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
	rows := displayRows(dirs)

	if *printOnly {
		fmt.Println(strings.Join(rows, "\n"))
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
		"ctrl-x:execute-silent(%s workspace remove --path {2} --quiet)+reload(%s workspace --print)",
		self, self,
	)
	togglePreview := "ctrl-/:toggle-preview"

	sel, err := fzfutil.Pick(rows, fzfutil.Options{
		Prompt:        "workspace> ",
		Header:        "ctrl-t: add worktree · ctrl-x: remove workspace · ctrl-/: toggle preview",
		Bindings:      []string{addBind, removeBind, togglePreview},
		Preview:       fmt.Sprintf("%s workspace preview --path {2}", self),
		PreviewWindow: "follow",
		Delimiter:     "\t",
		WithNth:       "1",
		ANSI:          true,
	})
	if err != nil || sel == "" {
		return
	}
	if err := session.Open(repo.TrimSlash(rowPath(sel)), session.OpenOptions{}); err != nil {
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
	if err := worktree.AddWorktree(repo.TrimSlash(sel)); err != nil {
		notify.Error(err.Error())
	}
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
		msg, err := worktree.Remove(path)
		if !*quiet {
			if err != nil {
				notify.Error(err.Error())
			} else {
				notify.Info(msg)
			}
		}
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
