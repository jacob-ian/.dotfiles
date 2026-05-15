package worktree

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"jmux/internal/fzfutil"
	"jmux/internal/gitctl"
	"jmux/internal/notify"
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
	bareRoot := gitctl.CommonDir(cwd)
	if bareRoot == "" {
		notify.Error("Not in a bare repo worktree")
		return "", false
	}

	candidates := repo.BareRepoWorktrees(bareRoot, true)
	if len(candidates) == 0 {
		notify.Info("No removable worktrees")
		return "", false
	}

	sel, err := fzfutil.Pick(candidates, fzfutil.Options{Prompt: "remove worktree> "})
	if err != nil || sel == "" {
		return "", false
	}
	return repo.TrimSlash(sel), true
}

func removeWorktree(path string, quiet bool) {
	bareRoot := repo.FindBareRoot(path)
	if bareRoot == "" {
		bareRoot = gitctl.CommonDir(path)
	}

	sessionName := session.Name(path)
	inCurrent := tmuxctl.CurrentSession() == sessionName

	if !fastRemove(path, bareRoot) {
		// Fallback to a synchronous git worktree remove. We're already in the
		// session being removed, so don't kill it first — git would lose track
		// of the cwd. After git succeeds we still need to handle the session.
		if err := gitctl.WorktreeRemove(bareRoot, path, true); err != nil {
			if !quiet {
				notify.Errorf("Failed to remove worktree '%s'", displayName(path, bareRoot))
			}
			return
		}
	}

	// Worktree is now gone from git's view. Kill the session — but if it's the
	// session we're inside, killing it directly would SIGHUP this process before
	// the picker can reload. Detach a worker that survives our death.
	if inCurrent {
		spawnDetached("tmux", "kill-session", "-t="+sessionName)
	} else {
		tmuxctl.KillSession(sessionName)
	}

	if !quiet {
		notify.Infof("Removed worktree '%s'", displayName(path, bareRoot))
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
	spawnDetached("rm", "-rf", trash)
	return true
}

// spawnDetached starts a session-leader subprocess with detached stdio; the
// child survives this process's SIGHUP.
func spawnDetached(name string, args ...string) {
	cmd := exec.Command(name, args...)
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
