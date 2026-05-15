package gitctl

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestCleanErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"strips fatal", errors.New("fatal: invalid reference: foo"), "invalid reference: foo"},
		{"trims whitespace", errors.New("  fatal: x  \n"), "x"},
		{"first line only", errors.New("fatal: first\nsecond\nthird"), "first"},
		{"no fatal prefix", errors.New("error: something"), "error: something"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanErr(tt.err); got != tt.want {
				t.Errorf("CleanErr(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestRefExists(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := initTestRepo(t)

	tests := []struct {
		name string
		ref  string
		want bool
	}{
		{"local branch exists", "local-only", true},
		{"remote-only resolves via origin/ prefix", "remote-only", true},
		{"present locally and on remote", "main", true},
		{"missing entirely", "ghost", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RefExists(repo, tt.ref); got != tt.want {
				t.Errorf("RefExists(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

// initTestRepo creates a tempdir git repo with:
//   - a `main` branch (local + at refs/remotes/origin/main)
//   - a `local-only` local branch
//   - a `remote-only` ref under refs/remotes/origin/
//
// Author/committer identity is set via env to avoid depending on user gitconfig.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("symbolic-ref", "HEAD", "refs/heads/main")
	run("commit", "--allow-empty", "-q", "-m", "init")
	run("branch", "local-only")
	run("update-ref", "refs/remotes/origin/remote-only", "HEAD")
	run("update-ref", "refs/remotes/origin/main", "HEAD")
	return dir
}
