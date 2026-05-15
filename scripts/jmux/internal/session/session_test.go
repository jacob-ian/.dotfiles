package session

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeBareRepo creates a directory that satisfies repo.IsBareRepo (HEAD file
// + refs/ subdir, no .git). Returns the bare-root path.
func fakeBareRepo(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestName(t *testing.T) {
	top := t.TempDir()
	parent := filepath.Join(top, "code")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	bare := fakeBareRepo(t, filepath.Join(parent, "myrepo"))

	wtSimple := filepath.Join(bare, "feature-a")
	wtNested := filepath.Join(bare, "feat", "foo")
	wtDot := filepath.Join(bare, "v1.2.3")
	nonRepo := filepath.Join(parent, "regular")
	for _, p := range []string{wtSimple, wtNested, wtDot, nonRepo} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"worktree of bare repo", wtSimple, "myrepo_feature-a"},
		{"nested worktree path", wtNested, "myrepo_feat_foo"},
		{"worktree name with dots", wtDot, "myrepo_v1_2_3"},
		{"bare repo itself uses parent_base fallback", bare, "code_myrepo"},
		{"non-repo dir uses parent_base fallback", nonRepo, "code_regular"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Name(tt.dir); got != tt.want {
				t.Errorf("Name(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}
