package worktree

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"jmux/internal/fzfutil"
	"jmux/internal/notify"
	"jmux/internal/repo"
	"jmux/internal/session"
)

// AllFeatureWorktrees collects non-default worktrees across every scan root.
func AllFeatureWorktrees() []string {
	roots := repo.ExpandPaths(repo.ScanRoots)
	return repo.ScanReposParallel(roots, repo.FeatureWorktrees)
}

// RunPicker handles `jmux worktree`. With --print, just lists worktree paths
// to stdout (used by fzf reload binding).
func RunPicker(args []string) {
	fs := flag.NewFlagSet("worktree", flag.ExitOnError)
	printOnly := fs.Bool("print", false, "Print worktree paths and exit")
	fs.Parse(args)

	dirs := AllFeatureWorktrees()

	if *printOnly {
		fmt.Println(strings.Join(dirs, "\n"))
		return
	}

	if len(dirs) == 0 {
		notify.Info("No worktrees found")
		return
	}

	self, err := os.Executable()
	if err != nil {
		self = "jmux"
	}

	bind := fmt.Sprintf(
		"ctrl-x:execute-silent(%s worktree remove --path {} --quiet)+reload(%s worktree --print)",
		self, self,
	)

	sel, err := fzfutil.Run(dirs, fzfutil.Options{
		Prompt:   "worktree> ",
		Header:   "ctrl-x: remove worktree",
		Bindings: []string{bind},
	})
	if err != nil || sel == "" {
		return
	}
	if err := session.Open(repo.TrimSlash(sel), session.OpenOptions{}); err != nil {
		notify.Error(err.Error())
	}
}
