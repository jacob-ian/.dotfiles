package session

import (
	"jmux/internal/fzfutil"
	"jmux/internal/notify"
	"jmux/internal/repo"
)

// AllDirs returns the candidate directories shown by the default picker:
// every worktree under each scan root, plus the additional and root dirs themselves.
func AllDirs() []string {
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

// Pick runs the default sessionizer fzf picker and returns the selected dir,
// or "" if the user canceled.
func Pick() string {
	dirs := AllDirs()
	if len(dirs) == 0 {
		return ""
	}
	sel, err := fzfutil.Pick(dirs, fzfutil.Options{Prompt: "session> "})
	if err != nil {
		return ""
	}
	return repo.TrimSlash(sel)
}

// RunPicker handles `jmux` (the default subcommand): picks a session dir
// and opens it.
func RunPicker() {
	dir := Pick()
	if dir == "" {
		return
	}
	if err := Open(dir, OpenOptions{}); err != nil {
		notify.Error(err.Error())
	}
}
