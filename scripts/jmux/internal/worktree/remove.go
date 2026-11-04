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

// IsManagedWorktree reports whether path is a removable worktree: under a bare
// repo's admin and not the default, main or master checkout.
func IsManagedWorktree(path string) bool {
	return managedBareRoot(path) != ""
}

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

// Remove takes a worktree out of git's view and kills its session, refusing
// anything that isn't a removable worktree so it never deletes a plain
// directory or a default checkout.
func Remove(cfg repo.Config, path string) (string, error) {
	bareRoot := managedBareRoot(path)
	if bareRoot == "" {
		return "", fmt.Errorf("refusing to remove '%s': not a removable worktree", filepath.Base(path))
	}

	harvested := ""
	if cfg.IsScratch(path) {
		branch, err := harvest(path, bareRoot)
		if err != nil {
			return "", fmt.Errorf("harvesting '%s': %w", displayName(path, bareRoot), err)
		}
		harvested = branch
	}

	if !fastRemove(path, bareRoot) {
		// The session is killed after git, not before: we may be running inside
		// it, and git would lose its cwd.
		if err := gitctl.WorktreeRemove(bareRoot, path, true); err != nil {
			return "", fmt.Errorf("failed to remove worktree '%s'", displayName(path, bareRoot))
		}
	}

	session.Kill(session.Name(path))

	if harvested != "" {
		// Deletable only now that no worktree has it checked out.
		_ = gitctl.DeleteBranch(bareRoot, harvested)
		return fmt.Sprintf("Harvested '%s' into %s", harvested, gitctl.DefaultBranch(bareRoot)), nil
	}
	return fmt.Sprintf("Removed worktree '%s'", displayName(path, bareRoot)), nil
}

// harvest commits the scratch worktree and squashes it onto the default
// checkout. Stray top-level files are moved into <branch>/ so the session
// still lands as one dated directory; edits to older notes stay put.
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

// fastRemove renames the worktree aside and deletes its admin entry, then
// detaches the slow rm -rf (antivirus scans every node_modules file) so the
// picker sees it gone immediately.
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

// spawnDetached starts a subprocess that survives this process's SIGHUP.
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
