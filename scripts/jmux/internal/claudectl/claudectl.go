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
)

type lockFile struct {
	WorkspaceFolders []string `json:"workspaceFolders"`
}

// Run sets CLAUDE_CODE_SSE_PORT (if a matching lock file is present) then
// tries `claude --continue args...`. If that exits non-zero (typically
// because no resumable session exists for cwd), replaces this process with
// a plain `claude args...`. Mirrors the shell idiom `claude --continue || claude`.
func Run(args []string) error {
	if port := findPort(); port != "" {
		os.Setenv("CLAUDE_CODE_SSE_PORT", port)
	}

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return err
	}

	cmd := exec.Command(claudePath, append([]string{"--continue"}, args...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err == nil {
		return nil
	}

	return syscall.Exec(claudePath, append([]string{claudePath}, args...), os.Environ())
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
