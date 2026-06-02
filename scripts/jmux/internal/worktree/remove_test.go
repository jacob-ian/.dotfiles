package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsManagedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare, plain := initBareWithWorktrees(t)

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"feature worktree is removable", filepath.Join(bare, "feature"), true},
		{"main worktree is protected", filepath.Join(bare, "main"), false},
		{"plain git repo is not a worktree", plain, false},
		{"nonexistent path under bare", filepath.Join(bare, "ghost"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsManagedWorktree(tt.path); got != tt.want {
				t.Errorf("IsManagedWorktree(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// initBareWithWorktrees builds jmux's bare-repo layout: a `--bare` clone with
// `main` and `feature` worktrees checked out beneath it. It returns the bare
// root and an unrelated plain (non-bare) repo path.
func initBareWithWorktrees(t *testing.T) (bare, plain string) {
	t.Helper()
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	git := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Resolve symlinks: on macOS t.TempDir() lives under /tmp -> /private/tmp,
	// and git records worktree gitdir paths in canonical form. Production picker
	// rows are likewise canonical, so match that here.
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	git(src, "init", "-q")
	git(src, "symbolic-ref", "HEAD", "refs/heads/main")
	git(src, "commit", "--allow-empty", "-q", "-m", "init")
	git(src, "branch", "feature")

	bare = filepath.Join(root, "repo")
	git(root, "clone", "-q", "--bare", src, bare)
	git(bare, "worktree", "add", "-q", "main", "main")
	git(bare, "worktree", "add", "-q", "feature", "feature")

	plain = filepath.Join(root, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	git(plain, "init", "-q")

	return bare, plain
}
