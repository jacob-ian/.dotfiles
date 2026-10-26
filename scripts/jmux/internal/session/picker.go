package session

import (
	"os"

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
	if scratch := os.ExpandEnv(repo.ScratchRoot); repo.IsDir(scratch) {
		dirs = append(dirs, repo.ProjectDirs(scratch)...)
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
	rows := make([]string, len(dirs))
	for i, d := range dirs {
		rows[i] = fzfutil.Row(repo.Marker(d)+d, d)
	}
	sel, err := fzfutil.Pick(rows, fzfutil.Options{Prompt: "session> ", Delimiter: "\t", WithNth: "1"})
	if err != nil || sel == "" {
		return nil
	}
	return Open(repo.TrimSlash(fzfutil.RowPath(sel)), OpenOptions{})
}
