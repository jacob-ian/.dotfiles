package worktree

import (
	"fmt"
	"os"
	"strings"

	"jmux/internal/fzfutil"
	"jmux/internal/notify"
	"jmux/internal/repo"
	"jmux/internal/session"
)

func AllFeatureWorktrees() []string {
	roots := repo.ExpandPaths(repo.ScanRoots)
	out := repo.ScanReposParallel(roots, repo.FeatureWorktrees)
	return append(out, repo.FeatureWorktrees(os.ExpandEnv(repo.ScratchRoot))...)
}

func rows(dirs []string) []string {
	out := make([]string, len(dirs))
	for i, d := range dirs {
		out[i] = fzfutil.Row(repo.Marker(d)+d, d)
	}
	return out
}

// RunItems handles `jmux fzf worktree items`: the picker's reload rows.
func RunItems() {
	fmt.Println(strings.Join(rows(AllFeatureWorktrees()), "\n"))
}

// RunPicker handles `jmux worktree`: the feature-worktrees picker.
func RunPicker() error {
	dirs := AllFeatureWorktrees()
	if len(dirs) == 0 {
		notify.Info("No worktrees found")
		return nil
	}

	self := fzfutil.Self()
	sel, err := fzfutil.Pick(rows(dirs), fzfutil.Options{
		Prompt: "worktree> ",
		Header: "ctrl-x: remove worktree · ctrl-/: toggle preview",
		Bindings: []string{
			fmt.Sprintf("ctrl-x:execute-silent(%s worktree remove --path {2} --quiet)+reload(%s fzf worktree items)", self, self),
			"ctrl-/:toggle-preview",
		},
		Preview:       fmt.Sprintf("%s fzf workspace preview --path {2}", self),
		PreviewWindow: "follow",
		Delimiter:     "\t",
		WithNth:       "1",
	})
	if err != nil || sel == "" {
		return nil
	}
	return session.Open(repo.TrimSlash(fzfutil.RowPath(sel)), session.OpenOptions{})
}
