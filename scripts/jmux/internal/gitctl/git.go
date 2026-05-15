package gitctl

import (
	"bufio"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// CommonDir resolves the bare repo root for a worktree at dir.
// Returns "" if dir is not inside a bare repo worktree.
func CommonDir(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(string(out))
	if gitDir == "" || gitDir == ".git" {
		return ""
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	abs, err := filepath.Abs(gitDir)
	if err != nil {
		return ""
	}
	return abs
}

// RemoteBranches lists `origin/`-stripped remote branch names, omitting HEAD.
func RemoteBranches(bareRoot string) ([]string, error) {
	cmd := exec.Command("git", "branch", "-r")
	cmd.Dir = bareRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var branches []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.Contains(line, "HEAD") {
			continue
		}
		line = strings.TrimPrefix(line, "origin/")
		branches = append(branches, line)
	}
	return branches, nil
}

// DefaultBranch returns the name of origin/HEAD's target branch (e.g. "main").
// Returns "" if it can't be determined.
func DefaultBranch(bareRoot string) string {
	cmd := exec.Command("git", "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	cmd.Dir = bareRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/")
}

// WorktreeAdd creates a worktree at path. If createBranch is true, the branch
// is created (-b); otherwise it must already exist.
func WorktreeAdd(bareRoot, path, branch string, createBranch bool) error {
	args := []string{"worktree", "add", path}
	if createBranch {
		args = append(args, "-b", branch)
	} else {
		args = append(args, branch)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = bareRoot
	return cmd.Run()
}

// CurrentBranch returns the short branch name HEAD points at, or "" if
// detached.
func CurrentBranch(dir string) string {
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// AheadBehind returns commits ahead and behind ref, evaluated from dir.
// Returns (0, 0, false) if the comparison fails (e.g. ref not present).
func AheadBehind(dir, ref string) (int, int, bool) {
	cmd := exec.Command("git", "rev-list", "--left-right", "--count", ref+"...HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, false
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return 0, 0, false
	}
	behind, err1 := strconv.Atoi(parts[0])
	ahead, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return ahead, behind, true
}

// ShortStatus returns `git status --short` output from dir.
func ShortStatus(dir string) string {
	cmd := exec.Command("git", "status", "--short")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

// LogOneline runs `git log --oneline -n N <revRange>` from dir.
func LogOneline(dir, revRange string, n int) string {
	cmd := exec.Command("git", "log", "--oneline", "-n", strconv.Itoa(n), revRange)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

// WorktreeRemove removes a worktree from git's view. Use force to drop
// uncommitted changes.
func WorktreeRemove(bareRoot, path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	cmd := exec.Command("git", args...)
	if bareRoot != "" {
		cmd.Dir = bareRoot
	}
	return cmd.Run()
}
