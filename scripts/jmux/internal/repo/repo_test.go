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

	t.Run("skip default named by HEAD and main", func(t *testing.T) {
		history := filepath.Join(bare, "history")
		writeAdmin(t, bare, "history", filepath.Join(history, ".git"))
		if err := os.WriteFile(filepath.Join(bare, "HEAD"), []byte("ref: refs/heads/history\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := BareRepoWorktrees(bare, true)
		want := []string{feat}
		if !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestDefaultBranchName(t *testing.T) {
	tests := map[string]struct {
		head string
		want string
	}{
		"branch":   {head: "ref: refs/heads/history\n", want: "history"},
		"detached": {head: "0123456789abcdef0123456789abcdef01234567\n", want: ""},
		"missing":  {want: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			bare := t.TempDir()
			if tt.head != "" {
				if err := os.WriteFile(filepath.Join(bare, "HEAD"), []byte(tt.head), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := DefaultBranchName(bare); got != tt.want {
				t.Errorf("DefaultBranchName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWorkDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	ScratchRoot = "$HOME/scratch"
	nested := filepath.Join(root, "scratch", "2026-10-26-foo", "2026-10-26-foo")
	bare := filepath.Join(root, "scratch", "2026-10-26-bare")
	other := filepath.Join(root, "euc", "repo", "repo")
	for _, d := range []string{nested, bare, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tests := map[string]struct{ dir, want string }{
		"scratch with dated dir":    {dir: filepath.Dir(nested), want: nested},
		"scratch without dated dir": {dir: bare, want: bare},
		"non-scratch same shape":    {dir: filepath.Dir(other), want: filepath.Dir(other)},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := WorkDir(tt.dir); got != tt.want {
				t.Errorf("WorkDir(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

func TestIsScratch(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	ScratchRoot = "$HOME/scratch"
	if err := os.MkdirAll(filepath.Join(root, "scratch", "2026-10-26"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "scratchpad"), 0o755); err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		path string
		want bool
	}{
		"root":         {path: filepath.Join(root, "scratch"), want: true},
		"worktree":     {path: filepath.Join(root, "scratch", "2026-10-26"), want: true},
		"sibling":      {path: filepath.Join(root, "scratchpad"), want: false},
		"nonexistent":  {path: filepath.Join(root, "scratch", "missing"), want: true},
		"outside home": {path: t.TempDir(), want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := IsScratch(tt.path); got != tt.want {
				t.Errorf("IsScratch(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
