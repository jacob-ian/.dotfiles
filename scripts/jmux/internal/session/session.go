package session

import (
	"fmt"
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
	// InstallCmd, if non-empty, runs in a detached "install" window. The shell
	// wrapper pauses on failure so the user can read the error.
	InstallCmd string
}

// Open ensures a tmux session exists for dir and switches/attaches to it.
func Open(dir string, opts OpenOptions) error {
	name := Name(dir)
	if !tmuxctl.HasSession(name) {
		if err := tmuxctl.NewSession(name, dir, "nvim", "nvim"); err != nil {
			return fmt.Errorf("create session %q: %w", name, err)
		}
		if opts.WithClaude {
			if err := tmuxctl.NewWindow(name, "claude", dir, "jmux claude", false); err != nil {
				return fmt.Errorf("create claude window: %w", err)
			}
			tmuxctl.SelectWindow(name + ":1")
		}
		if opts.InstallCmd != "" {
			shellCmd := fmt.Sprintf("%s || { echo; echo '[install failed — press enter to close]'; read; }", opts.InstallCmd)
			if err := tmuxctl.NewWindow(name, "install", dir, shellCmd, true); err != nil {
				return fmt.Errorf("create install window: %w", err)
			}
		}
	}

	if tmuxctl.InsideTmux() {
		return tmuxctl.SwitchClient(name)
	}
	return tmuxctl.Attach(name)
}
