package worktree_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"jmux/internal/repo"
	"jmux/internal/worktree"
)

// scratchRepo builds a remoteless bare repo with HEAD `history`, a history
// checkout holding old/x.md, and a session worktree; ScratchRoot points at it.
func scratchRepo(t *testing.T, session string) (bare, wt, history string, git func(dir string, args ...string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	git = func(dir string, args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(filepath.Join(src, "old"), 0o755); err != nil {
		t.Fatal(err)
	}
	git(src, "init", "-q")
	git(src, "symbolic-ref", "HEAD", "refs/heads/history")
	if err := os.WriteFile(filepath.Join(src, "old", "x.md"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(src, "add", "-A")
	git(src, "commit", "-q", "-m", "init")

	bare = filepath.Join(root, "scratch")
	git(root, "clone", "-q", "--bare", src, bare)
	git(bare, "remote", "remove", "origin")
	git(bare, "symbolic-ref", "HEAD", "refs/heads/history")
	git(bare, "worktree", "add", "-q", "history", "history")
	git(bare, "worktree", "add", "-q", "-b", session, session, "history")

	return bare, filepath.Join(bare, session), filepath.Join(bare, "history"), git
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveHarvestsScratch(t *testing.T) {
	const session = "2026-10-26-foo"

	t.Run("new files land in the session directory", func(t *testing.T) {
		bare, wt, history, git := scratchRepo(t, session)
		write(t, filepath.Join(wt, "notes.md"), "hi\n")
		write(t, filepath.Join(wt, "sub", "b.md"), "b\n")
		write(t, filepath.Join(wt, "old", "x.md"), "two\n")

		msg, err := worktree.Remove(repo.Config{ScratchRoot: bare}, wt)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(msg, "Harvested") {
			t.Errorf("message = %q", msg)
		}
		for _, f := range []string{
			filepath.Join(history, session, "notes.md"),
			filepath.Join(history, "sub", "b.md"),
		} {
			if _, err := os.Stat(f); err != nil {
				t.Errorf("missing %s", f)
			}
		}
		if got, _ := os.ReadFile(filepath.Join(history, "old", "x.md")); string(got) != "two\n" {
			t.Errorf("old/x.md = %q, want edited content", got)
		}
		if got := git(history, "log", "--format=%s", "-1"); got != session {
			t.Errorf("history head = %q, want %q", got, session)
		}
		if got := git(history, "status", "--porcelain"); got != "" {
			t.Errorf("history dirty after harvest:\n%s", got)
		}
		if out := git(bare, "branch", "--list", session); out != "" {
			t.Errorf("branch still exists: %q", out)
		}
		if _, err := os.Stat(filepath.Join(bare, "worktrees", session)); !os.IsNotExist(err) {
			t.Error("worktree admin entry still present")
		}
	})

	t.Run("empty session leaves history untouched", func(t *testing.T) {
		bare, wt, history, git := scratchRepo(t, session)
		before := git(history, "rev-parse", "HEAD")

		if _, err := worktree.Remove(repo.Config{ScratchRoot: bare}, wt); err != nil {
			t.Fatal(err)
		}
		if got := git(history, "rev-parse", "HEAD"); got != before {
			t.Errorf("history moved from %s to %s", before, got)
		}
		if out := git(bare, "branch", "--list", session); out != "" {
			t.Errorf("branch still exists: %q", out)
		}
	})

	t.Run("conflict keeps the worktree and a clean history", func(t *testing.T) {
		bare, wt, history, git := scratchRepo(t, session)
		write(t, filepath.Join(wt, "old", "x.md"), "mine\n")
		write(t, filepath.Join(history, "old", "x.md"), "theirs\n")
		git(history, "commit", "-q", "-am", "diverge")

		_, err := worktree.Remove(repo.Config{ScratchRoot: bare}, wt)
		if err == nil || !strings.Contains(err.Error(), "merging into history") {
			t.Fatalf("err = %v, want merge failure", err)
		}
		if _, statErr := os.Stat(filepath.Join(wt, "old", "x.md")); statErr != nil {
			t.Error("worktree was removed despite the conflict")
		}
		if got := git(history, "status", "--porcelain"); got != "" {
			t.Errorf("history left dirty:\n%s", got)
		}
		if got, _ := os.ReadFile(filepath.Join(history, "old", "x.md")); string(got) != "theirs\n" {
			t.Errorf("history content = %q, want untouched", got)
		}
	})
}
