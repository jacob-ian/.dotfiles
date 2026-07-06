package session

import (
	"jmux/internal/fzfutil"
	"jmux/internal/repo"
)

// allDirs returns the candidate directories shown by the default picker: every
// worktree under each scan root, plus the additional and root dirs themselves.
func allDirs() []string {
	roots := repo.ExpandPaths(repo.ScanRoots)
	dirs := repo.ScanReposParallel(roots, repo.ProjectDirs)
	for _, d := range repo.ExpandPaths(repo.AdditionalDirs) {
		if repo.IsDir(d) {
			dirs = append(dirs, d)
		}
	}
	for _, d := range roots {
		if repo.IsDir(d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// RunPicker handles `jmux` (the default subcommand): picks a session dir and
// opens it.
func RunPicker() error {
	dirs := allDirs()
	if len(dirs) == 0 {
		return nil
	}
	sel, err := fzfutil.Pick(dirs, fzfutil.Options{Prompt: "session> "})
	if err != nil || sel == "" {
		return nil
	}
	return Open(repo.TrimSlash(sel), OpenOptions{})
}
