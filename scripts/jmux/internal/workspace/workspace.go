// Package workspace is the overview of places you're actively working — every
// open tmux session unioned with every feature worktree — with bindings to add
// (ctrl-t) and remove (ctrl-x) them.
package workspace

import (
	"flag"
	"fmt"
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

// workspaces returns the overview rows: feature worktrees plus the directories
// of any open sessions, deduped by resolved path.
func workspaces() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = repo.TrimSlash(p)
		key := repo.Resolve(p)
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
// (column 2), so badges never leak into a path. Pane-carrying badges whose pane
// has died are dropped (their source is gone but never fired a cleanup event),
// and when several survive for one path each gets a "@window" label so
// concurrent claude agents are tellable apart.
func displayRows(dirs []string) []string {
	tagged := tag.All()
	panes := tmuxctl.PaneWindows()
	rows := make([]string, len(dirs))
	for i, d := range dirs {
		var live []tag.Badge
		paned := 0
		for _, b := range tagged[repo.Resolve(d)] {
			if b.Pane != "" {
				if _, ok := panes[b.Pane]; !ok {
					continue
				}
				paned++
			}
			live = append(live, b)
		}
		display := d
		if len(live) > 0 {
			parts := make([]string, len(live))
			for j, b := range live {
				parts[j] = b.Render()
				if b.Pane != "" && paned > 1 {
					parts[j] += " @" + panes[b.Pane]
				}
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

// RunItems handles `jmux fzf workspace items`: print the overview rows for the
// picker's reload bindings.
func RunItems() {
	fmt.Println(strings.Join(displayRows(workspaces()), "\n"))
}

// RunPicker handles `jmux workspace`: the fzf overview.
func RunPicker() error {
	dirs := workspaces()
	if len(dirs) == 0 {
		notify.Info("No workspaces found")
		return nil
	}

	self := fzfutil.Self()
	sel, err := fzfutil.Pick(displayRows(dirs), fzfutil.Options{
		Prompt: "workspace> ",
		Header: "ctrl-t: add worktree · ctrl-x: remove workspace · ctrl-/: toggle preview",
		Bindings: []string{
			fmt.Sprintf("ctrl-t:become(%s workspace add)", self),
			fmt.Sprintf("ctrl-x:execute-silent(%s workspace remove --path {2} --quiet)+reload(%s fzf workspace items)", self, self),
			"ctrl-/:toggle-preview",
		},
		Preview:       fmt.Sprintf("%s fzf workspace preview --path {2}", self),
		PreviewWindow: "follow",
		Delimiter:     "\t",
		WithNth:       "1",
		ANSI:          true,
	})
	if err != nil || sel == "" {
		return nil
	}
	return session.Open(repo.TrimSlash(rowPath(sel)), session.OpenOptions{})
}

// RunAdd handles `jmux workspace add`: pick a bare repo, then run the worktree
// branch flow against it.
func RunAdd() error {
	repos := repo.BareRepos()
	if len(repos) == 0 {
		notify.Info("No bare repos found")
		return nil
	}
	sel, err := fzfutil.Pick(repos, fzfutil.Options{Prompt: "repo> "})
	if err != nil || sel == "" {
		return nil
	}
	return worktree.AddWorktree(repo.TrimSlash(sel))
}

// RunRemove handles `jmux workspace remove --path P [--quiet]`. A worktree-backed
// workspace is removed (worktree + session); any other workspace just has its
// session closed — the directory is never deleted. --quiet suppresses the
// success messages; failures still propagate.
func RunRemove(args []string) error {
	fs := flag.NewFlagSet("workspace remove", flag.ExitOnError)
	pathArg := fs.String("path", "", "Workspace path to remove")
	quiet := fs.Bool("quiet", false, "Suppress tmux display-message status")
	fs.Parse(args)

	path := repo.TrimSlash(*pathArg)
	if path == "" {
		return nil
	}

	if worktree.IsManagedWorktree(path) {
		msg, err := worktree.Remove(path)
		if err != nil {
			return err
		}
		if !*quiet {
			notify.Info(msg)
		}
		return nil
	}

	name := session.Name(path)
	if !tmuxctl.HasSession(name) {
		if !*quiet {
			notify.Infof("No session for '%s'", filepath.Base(path))
		}
		return nil
	}
	// Killing the session takes claude down without a SessionEnd hook, so its
	// badges would outlive it — the directory stays, so path pruning never
	// catches them either.
	tag.UnsetPrefix(path, "claude")
	session.Kill(name)
	if !*quiet {
		notify.Infof("Closed workspace '%s'", filepath.Base(path))
	}
	return nil
}
