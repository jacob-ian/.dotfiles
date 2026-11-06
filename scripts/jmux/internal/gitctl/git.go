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

	"jmux/internal/repo"
)

// gitReadTimeout keeps a wedged git (e.g. an index lock) from hanging an fzf preview.
const gitReadTimeout = 5 * time.Second

func gitOut(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitReadTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// gitRun returns git's own stderr as the error message.
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

// CommonDir returns the bare repo root for the worktree at dir, or "".
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

// DefaultBranch is origin/HEAD's branch, falling back to the bare repo's own HEAD.
func DefaultBranch(bareRoot string) string {
	out, err := gitOut(bareRoot, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		return strings.TrimPrefix(strings.TrimSpace(out), "origin/")
	}
	return repo.DefaultBranchName(bareRoot)
}

// WorktreeAdd creates a worktree at path; with createBranch the branch is
// created from base (callers pass origin/<default>: the bare HEAD is usually stale).
func WorktreeAdd(bareRoot, path, branch, base string, createBranch bool) error {
	args := []string{"worktree", "add", path}
	if createBranch {
		args = append(args, "-b", branch)
	} else {
		args = append(args, branch)
	}
	if createBranch && base != "" {
		// --no-track: the upstream is set on first push, not to origin/<default>.
		args = append(args, "--no-track", base)
	}
	return gitRun(bareRoot, args...)
}

// FetchBranch updates origin/<branch> so a following worktree add tracks it.
func FetchBranch(bareRoot, branch string) error {
	return gitRun(bareRoot, "fetch", "origin", branch)
}

// FastForward errors rather than merging when the branches have diverged.
func FastForward(worktreeDir, ref string) error {
	return gitRun(worktreeDir, "merge", "--ff-only", ref)
}

// WorktreeForBranch returns the worktree with branch checked out, or "" (git
// forbids one branch in two trees, so callers reuse it).
func WorktreeForBranch(bareRoot, branch string) string {
	out, err := gitOut(bareRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return ""
	}
	return worktreeForBranch(out, branch)
}

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

// CleanErr collapses a git error to one line without the "fatal: " prefix.
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

// RefExists checks ref as-is and as origin/<ref>, matching worktree add's dwim.
func RefExists(dir, ref string) bool {
	for _, name := range []string{ref, "origin/" + ref} {
		if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", name); err == nil {
			return true
		}
	}
	return false
}

// RepoSlug returns "owner/repo" of dir's origin remote, or "".
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

// CurrentBranch returns HEAD's short branch name, or "" when detached.
func CurrentBranch(dir string) string {
	out, err := gitOut(dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// AheadBehind returns commits ahead and behind ref; false when ref doesn't resolve.
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

// ShortStatus is `git status --short`, leading column whitespace preserved.
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

// SetupBareRemote adds the fetch refspec, origin/* refs and origin/HEAD that
// `clone --bare` leaves out.
func SetupBareRemote(bareRoot string) error {
	if err := gitRun(bareRoot, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return err
	}
	if err := gitRun(bareRoot, "fetch", "origin"); err != nil {
		return err
	}
	return gitRun(bareRoot, "remote", "set-head", "origin", "--auto")
}

// StageAll is `git add -A`: untracked files included.
func StageAll(dir string) error {
	return gitRun(dir, "add", "-A")
}

func HasStaged(dir string) bool {
	return gitRun(dir, "diff", "--cached", "--quiet") != nil
}

// AddedSince lists files in dir's index (not HEAD) absent from ref's tree.
func AddedSince(dir, ref string) ([]string, error) {
	out, err := gitOut(dir, "diff", "--cached", "--name-only", "--diff-filter=A", ref)
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

func Move(dir, src, dst string) error {
	return gitRun(dir, "mv", src, dst)
}

func Commit(dir, msg string) error {
	return gitRun(dir, "commit", "-q", "-m", msg)
}

// MergeSquash leaves the index mid-merge on conflict; see ResetMerge.
func MergeSquash(dir, branch string) error {
	return gitRun(dir, "merge", "--squash", "-q", branch)
}

// ResetMerge abandons a squash merge (no MERGE_HEAD, so `merge --abort` can't)
// while keeping unrelated local edits.
func ResetMerge(dir string) error {
	return gitRun(dir, "reset", "-q", "--merge")
}

// DeleteBranch fails while any worktree has branch checked out.
func DeleteBranch(bareRoot, branch string) error {
	return gitRun(bareRoot, "branch", "-q", "-D", branch)
}

// WorktreeRemove removes a worktree; force drops uncommitted changes.
func WorktreeRemove(bareRoot, path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	return gitRun(bareRoot, args...)
}

// RevParse resolves ref to a commit hash; false when it doesn't exist.
func RevParse(dir, ref string) (string, bool) {
	out, err := gitOut(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(out), true
}

// IsClean ignores untracked files (a reset keeps them) and treats an
// unreadable status as dirty.
func IsClean(dir string) bool {
	out, err := gitOut(dir, "status", "--porcelain", "--untracked-files=no")
	return err == nil && strings.TrimSpace(out) == ""
}

func ResetHard(dir, ref string) error {
	return gitRun(dir, "reset", "-q", "--hard", ref)
}
