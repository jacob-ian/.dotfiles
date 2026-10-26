package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"jmux/internal/gitctl"
	"jmux/internal/repo"
	"jmux/internal/session"
)

// IsManagedWorktree reports whether path is a jmux-removable worktree: it lives
// under a bare repo's worktrees admin and is not the default/main branch
// checkout. Plain directories and main checkouts return false so callers know
// not to delete them.
func IsManagedWorktree(path string) bool {
	return managedBareRoot(path) != ""
}

// managedBareRoot returns the bare repo root when path is a jmux-removable
// worktree (see IsManagedWorktree), or "".
func managedBareRoot(path string) string {
	bareRoot := repo.FindBareRoot(path)
	if bareRoot == "" {
		bareRoot = gitctl.CommonDir(path)
	}
	if bareRoot == "" || repo.AdminDirFor(bareRoot, path) == "" {
		return ""
	}
	base := filepath.Base(path)
	if base == "main" || base == "master" || base == gitctl.DefaultBranch(bareRoot) {
		return ""
	}
	return bareRoot
}

// Remove takes a worktree out of git's view and kills its tmux session. It
// refuses any path that isn't a removable worktree (see IsManagedWorktree),
// so it never deletes a plain directory or the main/default checkout.
func Remove(cfg repo.Config, path string) (string, error) {
	bareRoot := managedBareRoot(path)
	if bareRoot == "" {
		return "", fmt.Errorf("refusing to remove '%s': not a removable worktree", filepath.Base(path))
	}

	// Scratch worktrees are sessions, not discardable branches.
	harvested := ""
	if cfg.IsScratch(path) {
		branch, err := harvest(path, bareRoot)
		if err != nil {
			return "", fmt.Errorf("harvesting '%s': %w", displayName(path, bareRoot), err)
		}
		harvested = branch
	}

	if !fastRemove(path, bareRoot) {
		// Fallback to a synchronous git worktree remove. We're already in the
		// session being removed, so don't kill it first — git would lose track
		// of the cwd. After git succeeds we still need to handle the session.
		if err := gitctl.WorktreeRemove(bareRoot, path, true); err != nil {
			return "", fmt.Errorf("failed to remove worktree '%s'", displayName(path, bareRoot))
		}
	}

	// Worktree is now gone from git's view; tear down its session.
	session.Kill(session.Name(path))

	if harvested != "" {
		// Deletable only now that no worktree has it checked out; the squash
		// commit holds its content, so a failure just leaves a stale ref.
		_ = gitctl.DeleteBranch(bareRoot, harvested)
		return fmt.Sprintf("Harvested '%s' into %s", harvested, gitctl.DefaultBranch(bareRoot)), nil
	}
	return fmt.Sprintf("Removed worktree '%s'", displayName(path, bareRoot)), nil
}

// harvest commits the scratch worktree and squashes it onto the default
// checkout, returning the branch for deletion once the worktree is gone. The
// session works inside <branch>/ already; any stray top-level files are moved
// in so it still lands as one dated directory. Edits to older notes stay put.
func harvest(path, bareRoot string) (string, error) {
	branch := gitctl.CurrentBranch(path)
	if branch == "" {
		return "", errors.New("detached HEAD")
	}
	def := gitctl.DefaultBranch(bareRoot)
	if def == "" {
		return "", errors.New("no default branch")
	}
	defWt := filepath.Join(bareRoot, def)
	if !repo.IsDir(defWt) {
		return "", fmt.Errorf("no %s checkout to harvest into", def)
	}

	if err := gitctl.StageAll(path); err != nil {
		return "", fmt.Errorf("staging: %s", gitctl.CleanErr(err))
	}
	added, err := gitctl.AddedSince(path, def)
	if err != nil {
		return "", fmt.Errorf("listing new files: %s", gitctl.CleanErr(err))
	}
	for _, f := range added {
		if strings.Contains(f, "/") || strings.HasPrefix(f, ".") {
			continue
		}
		if err := os.MkdirAll(filepath.Join(path, branch), 0o755); err != nil {
			return "", err
		}
		if err := gitctl.Move(path, f, filepath.Join(branch, f)); err != nil {
			return "", fmt.Errorf("moving %s: %s", f, gitctl.CleanErr(err))
		}
	}
	if gitctl.HasStaged(path) {
		if err := gitctl.Commit(path, branch); err != nil {
			return "", fmt.Errorf("committing: %s", gitctl.CleanErr(err))
		}
	}

	if err := gitctl.MergeSquash(defWt, branch); err != nil {
		_ = gitctl.ResetMerge(defWt)
		return "", fmt.Errorf("merging into %s: %s", def, gitctl.CleanErr(err))
	}
	if gitctl.HasStaged(defWt) {
		if err := gitctl.Commit(defWt, branch); err != nil {
			return "", fmt.Errorf("committing to %s: %s", def, gitctl.CleanErr(err))
		}
	}
	return branch, nil
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
