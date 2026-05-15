package gitctl

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// gitOut runs `git args...` in dir and returns stdout untrimmed.
// Stderr is discarded — use gitRun when the caller needs to surface it.
func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// gitRun runs `git args...` in dir, returning an error whose message is git's
// own stderr (so callers can pattern-match on or display it).
func gitRun(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

// CommonDir resolves the bare repo root for a worktree at dir.
// Returns "" if dir is not inside a bare repo worktree.
func CommonDir(dir string) string {
	out, err := gitOut(dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(out)
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
	out, err := gitOut(bareRoot, "branch", "-r")
	if err != nil {
		return nil, err
	}
	var branches []string
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "HEAD") {
			continue
		}
		branches = append(branches, strings.TrimPrefix(line, "origin/"))
	}
	return branches, nil
}

// DefaultBranch returns the name of origin/HEAD's target branch (e.g. "main").
// Returns "" if it can't be determined.
func DefaultBranch(bareRoot string) string {
	out, err := gitOut(bareRoot, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(out), "origin/")
}

// WorktreeAdd creates a worktree at path. If createBranch is true, the branch
// is created (-b); otherwise it must already exist locally or be reachable as
// origin/<branch> for dwim. On failure the returned error carries git's stderr.
func WorktreeAdd(bareRoot, path, branch string, createBranch bool) error {
	args := []string{"worktree", "add", path}
	if createBranch {
		args = append(args, "-b", branch)
	} else {
		args = append(args, branch)
	}
	return gitRun(bareRoot, args...)
}

// CleanErr collapses a git error to a single line, dropping git's "fatal: "
// prefix. Useful for status-bar style notifications where the full multi-line
// stderr would overflow.
func CleanErr(err error) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(err.Error())
	s = strings.TrimPrefix(s, "fatal: ")
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

// RefExists reports whether ref resolves in the repo at dir. It checks ref
// as-is and then prefixed with `origin/`, matching the dwim that
// `git worktree add <path> <ref>` performs when only a remote tracking branch
// exists.
func RefExists(dir, ref string) bool {
	for _, name := range []string{ref, "origin/" + ref} {
		cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", name)
		cmd.Dir = dir
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}

// CurrentBranch returns the short branch name HEAD points at, or "" if
// detached.
func CurrentBranch(dir string) string {
	out, err := gitOut(dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// AheadBehind returns commits ahead and behind ref, evaluated from dir.
// Returns (0, 0, false) if the comparison fails (e.g. ref not present).
func AheadBehind(dir, ref string) (int, int, bool) {
	out, err := gitOut(dir, "rev-list", "--left-right", "--count", ref+"...HEAD")
	if err != nil {
		return 0, 0, false
	}
	parts := strings.Fields(strings.TrimSpace(out))
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

// ShortStatus returns `git status --short` output from dir. The leading
// column whitespace (e.g. " M file") is preserved.
func ShortStatus(dir string) string {
	out, err := gitOut(dir, "status", "--short")
	if err != nil {
		return ""
	}
	return strings.TrimRight(out, "\n")
}

func LogOneline(dir, revRange string, n int) string {
	out, err := gitOut(dir, "log", "--oneline", "-n", strconv.Itoa(n), revRange)
	if err != nil {
		return ""
	}
	return strings.TrimRight(out, "\n")
}

// WorktreeRemove removes a worktree from git's view. Use force to drop
// uncommitted changes. bareRoot may be "" to run in the current process cwd.
func WorktreeRemove(bareRoot, path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	return gitRun(bareRoot, args...)
}
