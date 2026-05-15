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

func AllFeatureWorktrees() []string {
	roots := repo.ExpandPaths(repo.ScanRoots)
	return repo.ScanReposParallel(roots, repo.FeatureWorktrees)
}

// RunPicker with --print just lists worktree paths to stdout; this form is
// what the ctrl-x remove binding reloads against.
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

	removeBind := fmt.Sprintf(
		"ctrl-x:execute-silent(%s worktree remove --path {} --quiet)+reload(%s worktree --print)",
		self, self,
	)
	togglePreview := "ctrl-/:toggle-preview"

	sel, err := fzfutil.Pick(dirs, fzfutil.Options{
		Prompt:        "worktree> ",
		Header:        "ctrl-x: remove worktree · ctrl-/: toggle preview",
		Bindings:      []string{removeBind, togglePreview},
		Preview:       fmt.Sprintf("%s worktree preview --path {}", self),
		PreviewWindow: "follow",
	})
	if err != nil || sel == "" {
		return
	}
	if err := session.Open(repo.TrimSlash(sel), session.OpenOptions{}); err != nil {
		notify.Error(err.Error())
	}
}
