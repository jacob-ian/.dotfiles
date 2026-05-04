package session

import (
	"jmux/internal/fzfutil"
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

// Pick runs the default sessionizer fzf picker.
func Pick() string {
	dirs := AllDirs()
	if len(dirs) == 0 {
		return ""
	}
	sel, err := fzfutil.Run(dirs, fzfutil.Options{Prompt: "session> "})
	if err != nil {
		return ""
	}
	return repo.TrimSlash(sel)
}
