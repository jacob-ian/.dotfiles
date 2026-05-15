// Package claudectl launches `claude` with CLAUDE_CODE_SSE_PORT preset so
// that the new claude process auto-pairs with whichever Neovim instance
// owns the current worktree (via claudecode.nvim's lock files).
package claudectl

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"jmux/internal/tmuxctl"
)

type lockFile struct {
	WorkspaceFolders []string `json:"workspaceFolders"`
}

// Run sets CLAUDE_CODE_SSE_PORT (if a matching lock file is present), then
// tries `claude --continue args...` when this is the first claude window in
// the current tmux session. Subsequent claude windows skip --continue so the
// user gets a fresh session rather than a resume of the existing one. On
// --continue failure or when skipped, replaces this process with plain
// `claude args...`.
func Run(args []string) error {
	if port := findPort(); port != "" {
		os.Setenv("CLAUDE_CODE_SSE_PORT", port)
	}

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return err
	}

	if shouldContinue() {
		cmd := exec.Command(claudePath, append([]string{"--continue"}, args...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return syscall.Exec(claudePath, append([]string{claudePath}, args...), os.Environ())
}

// shouldContinue reports whether to attempt `claude --continue`. It returns
// false when another claude window already exists in the current tmux session
// (i.e. this is at least the second claude launch); outside tmux it returns
// true so a manual `jmux claude` invocation resumes as before.
func shouldContinue() bool {
	session := tmuxctl.CurrentSession()
	if session == "" {
		return true
	}
	others := tmuxctl.CountWindows(session, "claude")
	if tmuxctl.CurrentWindow() == "claude" {
		others--
	}
	return others <= 0
}

func findPort() string {
	ideDir := ideDir()
	entries, err := os.ReadDir(ideDir)
	if err != nil {
		return ""
	}
	pwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	pwd, err = filepath.EvalSymlinks(pwd)
	if err != nil {
		return ""
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".lock") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ideDir, name))
		if err != nil {
			continue
		}
		var lf lockFile
		if err := json.Unmarshal(data, &lf); err != nil {
			continue
		}
		for _, ws := range lf.WorkspaceFolders {
			if isAncestorOrEqual(ws, pwd) {
				return strings.TrimSuffix(name, ".lock")
			}
		}
	}
	return ""
}

func ideDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "ide")
	}
	return filepath.Join(os.Getenv("HOME"), ".claude", "ide")
}

func isAncestorOrEqual(ancestor, descendant string) bool {
	if ancestor == descendant {
		return true
	}
	return strings.HasPrefix(descendant, ancestor+string(filepath.Separator))
}
