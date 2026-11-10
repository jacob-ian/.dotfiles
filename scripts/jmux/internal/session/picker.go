package session

import (
	"errors"
	"fmt"
	"path/filepath"

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

// RunOpen handles `jmux open <dir>`.
func RunOpen(cfg repo.Config, args []string) error {
	if len(args) != 1 || args[0] == "" {
		return errors.New("usage: jmux open <dir>\n\nSwitches the tmux client to <dir>'s session, creating it (with nvim) if needed.")
	}
	dir, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	dir = repo.TrimSlash(dir)
	if !repo.IsDir(dir) {
		return fmt.Errorf("%s is not a directory", args[0])
	}
	return Open(dir, OpenOptions{Dir: cfg.WorkDir(dir)})
}
