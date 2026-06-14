package session

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

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
	// EditorCmd overrides the editor-window command; empty means plain "nvim".
	EditorCmd string
}

// Open ensures a tmux session exists for dir and switches/attaches to it.
func Open(dir string, opts OpenOptions) error {
	name := Name(dir)
	if !tmuxctl.HasSession(name) {
		editorCmd := opts.EditorCmd
		if editorCmd == "" {
			editorCmd = "nvim"
		}
		if err := tmuxctl.NewSession(name, dir, "nvim", editorCmd); err != nil {
			return fmt.Errorf("create session %q: %w", name, err)
		}
		// Stamp the originating dir so the workspace overview can map sessions
		// back to directories without reverse-engineering the session name.
		tmuxctl.SetSessionOption(name, "@jmux_dir", dir)
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

// Kill terminates the named tmux session. When it's the session we're currently
// inside, killing it directly would SIGHUP this process before any follow-up
// (e.g. an fzf reload) can run, so the kill is detached to a process that
// survives our death.
func Kill(name string) {
	if tmuxctl.CurrentSession() == name {
		cmd := exec.Command("tmux", "kill-session", "-t="+name)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		_ = cmd.Start()
		return
	}
	tmuxctl.KillSession(name)
}
