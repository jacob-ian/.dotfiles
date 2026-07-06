package worktree

import (
	"fmt"
	"strings"

	"jmux/internal/fzfutil"
	"jmux/internal/notify"
	"jmux/internal/repo"
	"jmux/internal/session"
)

func AllFeatureWorktrees() []string {
	roots := repo.ExpandPaths(repo.ScanRoots)
	return repo.ScanReposParallel(roots, repo.FeatureWorktrees)
}

// RunItems handles `jmux fzf worktree items`: print worktree paths for the
// picker's reload binding.
func RunItems() {
	fmt.Println(strings.Join(AllFeatureWorktrees(), "\n"))
}

// RunPicker handles `jmux worktree`: the feature-worktrees picker.
func RunPicker() error {
	dirs := AllFeatureWorktrees()
	if len(dirs) == 0 {
		notify.Info("No worktrees found")
		return nil
	}

	self := fzfutil.Self()
	sel, err := fzfutil.Pick(dirs, fzfutil.Options{
		Prompt: "worktree> ",
		Header: "ctrl-x: remove worktree · ctrl-/: toggle preview",
		Bindings: []string{
			fmt.Sprintf("ctrl-x:execute-silent(%s worktree remove --path {} --quiet)+reload(%s fzf worktree items)", self, self),
			"ctrl-/:toggle-preview",
		},
		Preview:       fmt.Sprintf("%s fzf workspace preview --path {}", self),
		PreviewWindow: "follow",
	})
	if err != nil || sel == "" {
		return nil
	}
	return session.Open(repo.TrimSlash(sel), session.OpenOptions{})
}
