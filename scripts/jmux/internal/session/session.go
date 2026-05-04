package session

import (
	"path/filepath"
	"strings"

	"jmux/internal/repo"
	"jmux/internal/tmuxctl"
)

func Name(dir string) string {
	if bareRoot := repo.FindBareRoot(dir); bareRoot != "" && bareRoot != dir {
		rel, err := filepath.Rel(bareRoot, dir)
		if err != nil {
			rel = filepath.Base(dir)
		}
		name := filepath.Base(bareRoot) + "_" + rel
		name = strings.ReplaceAll(name, "/", "_")
		return strings.ReplaceAll(name, ".", "_")
	}
	parent := filepath.Base(filepath.Dir(dir))
	base := filepath.Base(dir)
	return strings.ReplaceAll(parent+"_"+base, ".", "_")
}

type OpenOptions struct {
	WithClaude bool
}

// Open ensures a tmux session exists for dir and switches/attaches to it.
func Open(dir string, opts OpenOptions) {
	name := Name(dir)
	if !tmuxctl.HasSession(name) {
		tmuxctl.NewSession(name, dir, "nvim", "nvim")
		if opts.WithClaude {
			tmuxctl.NewWindow(name, "claude", dir, "claude", false)
			tmuxctl.SelectWindow(name + ":1")
		}
	}

	if tmuxctl.InsideTmux() {
		tmuxctl.SwitchClient(name)
		return
	}
	tmuxctl.Attach(name)
}
