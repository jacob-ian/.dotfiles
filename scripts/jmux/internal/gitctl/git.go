package gitctl

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// gitReadTimeout bounds the read-only git calls below so a wedged one (e.g.
// blocked on an index lock) can't hang an fzf preview process.
const gitReadTimeout = 5 * time.Second

// gitOut runs read-only `git args...` in dir (bounded by gitReadTimeout) and
// returns stdout untrimmed. Stderr is discarded; writes go through gitRun.
func gitOut(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitReadTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
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

// WorktreeAdd creates a worktree at path. If createBranch is true, the branch is
// created (-b) from base — or the bare repo's HEAD when base is "", which is
// usually a stale local default, so callers pass origin/<default> to start from
// the latest. Otherwise branch must already exist locally or be reachable as
// origin/<branch> for dwim. On failure the returned error carries git's stderr.
func WorktreeAdd(bareRoot, path, branch, base string, createBranch bool) error {
	args := []string{"worktree", "add", path}
	if createBranch {
		args = append(args, "-b", branch)
	} else {
		args = append(args, branch)
	}
	if createBranch && base != "" {
		// --no-track so branching off origin/<default> doesn't set the new branch's
		// upstream to the default; it gets one when first pushed.
		args = append(args, "--no-track", base)
	}
	return gitRun(bareRoot, args...)
}

// FetchBranch fetches branch from origin, updating origin/<branch> so a
// following worktree-add creates a local branch that tracks it.
func FetchBranch(bareRoot, branch string) error {
	return gitRun(bareRoot, "fetch", "origin", branch)
}

// WorktreeForBranch returns the path of the worktree that has branch checked
// out, or "" if none does — so an existing checkout is reused rather than
// failing a second `worktree add` (git forbids the same branch in two trees).
func WorktreeForBranch(bareRoot, branch string) string {
	out, err := gitOut(bareRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return ""
	}
	return worktreeForBranch(out, branch)
}

// worktreeForBranch scans `git worktree list --porcelain` output for the path
// whose entry checks out branch.
func worktreeForBranch(porcelain, branch string) string {
	want := "branch refs/heads/" + branch
	var path string
	for line := range strings.SplitSeq(porcelain, "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			path = p
		} else if line == want {
			return path
		}
	}
	return ""
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
		if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", name); err == nil {
			return true
		}
	}
	return false
}

// RepoSlug returns the "owner/repo" of dir's origin remote, or "". It accepts
// the scp (git@host:owner/repo), https, and ssh:// URL forms.
func RepoSlug(dir string) string {
	out, err := gitOut(dir, "config", "--get", "remote.origin.url")
	if err != nil {
		return ""
	}
	url := strings.TrimSuffix(strings.TrimSpace(out), ".git")
	url = strings.ReplaceAll(url, ":", "/")
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
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

// CloneBare clones url as a bare repo at dest, running from dir.
func CloneBare(dir, url, dest string) error {
	return gitRun(dir, "clone", "--bare", url, dest)
}

// SetupBareRemote gives a fresh bare clone the remote plumbing the rest of
// jmux assumes: a fetch refspec (clone --bare sets none, leaving FetchBranch
// inert), populated origin/* refs, and origin/HEAD (what DefaultBranch reads).
func SetupBareRemote(bareRoot string) error {
	if err := gitRun(bareRoot, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return err
	}
	if err := gitRun(bareRoot, "fetch", "origin"); err != nil {
		return err
	}
	return gitRun(bareRoot, "remote", "set-head", "origin", "--auto")
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
