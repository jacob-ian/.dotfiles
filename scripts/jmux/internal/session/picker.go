package session

import (
	"jmux/internal/fzfutil"
	"jmux/internal/repo"
)

// RunPicker handles bare `jmux`: pick a project dir and open its session.
func RunPicker(cfg repo.Config) error {
	dirs := cfg.ProjectDirs()
	if len(dirs) == 0 {
		return nil
	}
	rows := make([]string, len(dirs))
	for i, d := range dirs {
		rows[i] = fzfutil.Row(cfg.Marker(d)+d, d)
	}
	sel, err := fzfutil.Pick(rows, fzfutil.Options{Prompt: "session> ", Delimiter: "\t", WithNth: "1"})
	if err != nil || sel == "" {
		return nil
	}
	dir := repo.TrimSlash(fzfutil.RowPath(sel))
	return Open(dir, OpenOptions{Dir: cfg.WorkDir(dir)})
}
