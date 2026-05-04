package worktree

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"jmux/internal/fzfutil"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/tmuxctl"
)

// RunRemove handles `jmux worktree remove`.
//   - With --path P: skip fzf and target P directly.
//   - With --quiet:  suppress tmux display-message.
//   - Without flags: open an fzf picker rooted at the bare repo of cwd.
func RunRemove(args []string) {
	fs := flag.NewFlagSet("worktree remove", flag.ExitOnError)
	pathArg := fs.String("path", "", "Worktree path to remove (skips fzf)")
	quiet := fs.Bool("quiet", false, "Suppress tmux display-message status")
	fs.Parse(args)

	target := *pathArg
	if target == "" {
		t, ok := pickRemoveTarget()
		if !ok {
			return
		}
		target = t
	}

	removeWorktree(target, *quiet)
}

func pickRemoveTarget() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	bareRoot := repo.GitCommonDir(cwd)
	if bareRoot == "" {
		tmuxctl.DisplayMessage("Not in a bare repo worktree")
		return "", false
	}

	candidates := repo.BareRepoWorktrees(bareRoot, true)
	if len(candidates) == 0 {
		tmuxctl.DisplayMessage("No removable worktrees")
		return "", false
	}

	sel, err := fzfutil.Run(candidates, fzfutil.Options{Prompt: "remove worktree> "})
	if err != nil || sel == "" {
		return "", false
	}
	return repo.TrimSlash(sel), true
}

func removeWorktree(path string, quiet bool) {
	bareRoot := repo.FindBareRoot(path)
	if bareRoot == "" {
		bareRoot = repo.GitCommonDir(path)
	}

	sessionName := session.Name(path)
	tmuxctl.KillSession(sessionName)

	cmd := exec.Command("git", "worktree", "remove", path, "--force")
	if bareRoot != "" {
		cmd.Dir = bareRoot
	}
	if err := cmd.Run(); err != nil {
		if !quiet {
			tmuxctl.DisplayMessage(fmt.Sprintf("Failed to remove worktree '%s'", displayName(path, bareRoot)))
		}
		return
	}
	if !quiet {
		tmuxctl.DisplayMessage(fmt.Sprintf("Removed worktree '%s'", displayName(path, bareRoot)))
	}
}

func displayName(path, bareRoot string) string {
	if bareRoot == "" {
		return path
	}
	rel, err := filepath.Rel(bareRoot, path)
	if err != nil {
		return path
	}
	return rel
}
