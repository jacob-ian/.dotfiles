// Package workspace is the overview picker: every open tmux session unioned
// with every feature worktree.
package workspace

import (
	"cmp"
	"flag"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"jmux/internal/fzfutil"
	"jmux/internal/notify"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/statusbox"
	"jmux/internal/tag"
	"jmux/internal/tmuxctl"
	"jmux/internal/worktree"
)

func workspaces(cfg repo.Config) []string {
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
	for _, w := range cfg.FeatureWorktrees() {
		add(w)
	}
	for _, d := range tmuxctl.SessionDirs() {
		add(d)
	}
	slices.Sort(out)
	return out
}

// displayRows renders each path as "<path + badges>\t<path>". Badges whose pane
// has died are dropped, since their source never fired a cleanup event.
func displayRows(cfg repo.Config, dirs []string) []string {
	tagged := tag.All()
	panes := tmuxctl.PaneWindows()
	rows := make([]string, len(dirs))
	for i, d := range dirs {
		var live []tag.Tag
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
		slices.SortStableFunc(live, func(a, b tag.Tag) int {
			return cmp.Compare(windowIndex(a, panes), windowIndex(b, panes))
		})
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
		rows[i] = fzfutil.Row(cfg.Marker(d)+display, d)
	}
	return rows
}

// windowIndex orders badges like the tmux status line; pane-less tags sort first.
func windowIndex(b tag.Tag, panes map[string]string) int {
	if b.Pane == "" {
		return -1
	}
	n, err := strconv.Atoi(panes[b.Pane])
	if err != nil {
		return -1
	}
	return n
}

// RunItems handles `jmux fzf workspace items`, printing rows for fzf's reload bindings.
func RunItems(cfg repo.Config) {
	fmt.Println(strings.Join(displayRows(cfg, workspaces(cfg)), "\n"))
}

// RunPicker handles `jmux workspace`.
func RunPicker(cfg repo.Config) error {
	dirs := workspaces(cfg)
	if len(dirs) == 0 {
		notify.Info("No workspaces found")
		return nil
	}

	self := fzfutil.Self()
	sel, err := fzfutil.Pick(displayRows(cfg, dirs), fzfutil.Options{
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
	dir := repo.TrimSlash(fzfutil.RowPath(sel))
	return session.Open(dir, session.OpenOptions{Dir: cfg.WorkDir(dir)})
}

// RunAdd handles `jmux workspace add`.
func RunAdd(cfg repo.Config) error {
	repos := cfg.BareRepos()
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

// RunRemove handles `jmux workspace remove --path P [--quiet]`. Worktrees are
// removed; any other workspace only has its session closed.
func RunRemove(cfg repo.Config, args []string) error {
	fs := flag.NewFlagSet("workspace remove", flag.ExitOnError)
	pathArg := fs.String("path", "", "Workspace path to remove")
	quiet := fs.Bool("quiet", false, "Suppress tmux display-message status")
	fs.Parse(args)

	path := repo.TrimSlash(*pathArg)
	if path == "" {
		return nil
	}

	if worktree.IsManagedWorktree(path) {
		msg, err := worktree.Remove(cfg, path)
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
	// badges would outlive it.
	tag.UnsetPrefix(path, "claude")
	session.Kill(name)
	statusbox.Publish()
	if !*quiet {
		notify.Infof("Closed workspace '%s'", filepath.Base(path))
	}
	return nil
}
