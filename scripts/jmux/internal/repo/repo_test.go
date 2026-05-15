package repo

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// fakeBare creates a directory that looks like a bare repo's `worktrees/`
// parent. It returns the bare-root path; admin entries are added via writeAdmin.
func fakeBare(t *testing.T) string {
	t.Helper()
	bare := t.TempDir()
	if err := os.MkdirAll(filepath.Join(bare, "worktrees"), 0o755); err != nil {
		t.Fatal(err)
	}
	return bare
}

// writeAdmin creates a `<bare>/worktrees/<name>/gitdir` file pointing at
// gitdirPath (typically `<worktreePath>/.git`). Returns the admin dir path.
func writeAdmin(t *testing.T, bare, name, gitdirPath string) string {
	t.Helper()
	adminDir := filepath.Join(bare, "worktrees", name)
	if err := os.MkdirAll(adminDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(adminDir, "gitdir"), []byte(gitdirPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return adminDir
}

func TestAdminDirFor(t *testing.T) {
	bare := fakeBare(t)
	wtA := filepath.Join(bare, "feat-a")
	wtB := filepath.Join(bare, "feat-b")
	adminA := writeAdmin(t, bare, "feat-a", filepath.Join(wtA, ".git"))
	writeAdmin(t, bare, "feat-b", filepath.Join(wtB, ".git"))

	t.Run("match", func(t *testing.T) {
		if got := AdminDirFor(bare, wtA); got != adminA {
			t.Errorf("AdminDirFor(wtA) = %q, want %q", got, adminA)
		}
	})

	t.Run("no match", func(t *testing.T) {
		if got := AdminDirFor(bare, filepath.Join(bare, "missing")); got != "" {
			t.Errorf("AdminDirFor(missing) = %q, want \"\"", got)
		}
	})

	t.Run("bare with no worktrees dir", func(t *testing.T) {
		empty := t.TempDir()
		if got := AdminDirFor(empty, wtA); got != "" {
			t.Errorf("AdminDirFor(no worktrees dir) = %q, want \"\"", got)
		}
	})

	t.Run("admin entry with missing gitdir is skipped", func(t *testing.T) {
		dir := fakeBare(t)
		// Admin dir exists but no gitdir file.
		if err := os.MkdirAll(filepath.Join(dir, "worktrees", "broken"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := AdminDirFor(dir, filepath.Join(dir, "broken")); got != "" {
			t.Errorf("AdminDirFor(broken) = %q, want \"\"", got)
		}
	})
}

func TestBareRepoWorktrees(t *testing.T) {
	bare := fakeBare(t)
	main := filepath.Join(bare, "main")
	feat := filepath.Join(bare, "feature")
	writeAdmin(t, bare, "main", filepath.Join(main, ".git"))
	writeAdmin(t, bare, "feature", filepath.Join(feat, ".git"))

	t.Run("include default", func(t *testing.T) {
		got := BareRepoWorktrees(bare, false)
		slices.Sort(got)
		want := []string{feat, main}
		if !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("skip default", func(t *testing.T) {
		got := BareRepoWorktrees(bare, true)
		want := []string{feat}
		if !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}
