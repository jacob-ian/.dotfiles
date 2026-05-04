package worktree

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

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
	inCurrent := tmuxctl.CurrentSession() == sessionName

	if !fastRemove(path, bareRoot) {
		// Fallback to a synchronous git worktree remove. We're already in the
		// session being removed, so don't kill it first — git would lose track
		// of the cwd. After git succeeds we still need to handle the session.
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
	}

	// Worktree is now gone from git's view. Kill the session — but if it's the
	// session we're inside, killing it directly would SIGHUP this process before
	// the picker can reload. Detach a worker that survives our death.
	if inCurrent {
		spawnDetachedKillSession(sessionName)
	} else {
		tmuxctl.KillSession(sessionName)
	}

	if !quiet {
		tmuxctl.DisplayMessage(fmt.Sprintf("Removed worktree '%s'", displayName(path, bareRoot)))
	}
}

// fastRemove takes the worktree out of git's view in O(1) and detaches the
// recursive deletion to a background process. Returns true on success.
//
// Slowness on work machines comes from `rm -rf` walking node_modules etc.
// while antivirus scans every file. By renaming the working tree first
// (atomic on the same filesystem) and removing the small admin entry under
// <bareRoot>/worktrees/<name>, the picker reload sees the entry gone
// immediately and the user isn't blocked.
func fastRemove(path, bareRoot string) bool {
	if bareRoot == "" {
		return false
	}
	adminDir := repo.AdminDirFor(bareRoot, path)
	if adminDir == "" {
		return false
	}

	trash := fmt.Sprintf("%s.jmux-trash-%d", path, os.Getpid())
	if err := os.Rename(path, trash); err != nil {
		return false
	}
	if err := os.RemoveAll(adminDir); err != nil {
		os.Rename(trash, path)
		return false
	}
	spawnDetachedRm(trash)
	return true
}

func spawnDetachedRm(target string) {
	cmd := exec.Command("rm", "-rf", target)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Start()
}

// spawnDetachedKillSession fires `tmux kill-session` from a session-leader
// subprocess so the kill survives our own SIGHUP when we're inside the
// session being killed.
func spawnDetachedKillSession(name string) {
	cmd := exec.Command("tmux", "kill-session", "-t="+name)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Start()
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
