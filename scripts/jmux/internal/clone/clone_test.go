package clone

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"jmux/internal/gitctl"
	"jmux/internal/repo"
)

func TestRepoName(t *testing.T) {
	cases := map[string]string{
		"git@github.com:owner/repo.git":       "repo",
		"https://github.com/owner/repo":       "repo",
		"https://github.com/owner/repo.git/":  "repo",
		"ssh://git@github.com/owner/repo.git": "repo",
		"repo":                                "repo",
		"":                                    "",
	}
	for in, want := range cases {
		if got := repoName(in); got != want {
			t.Errorf("repoName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestClone runs the full clone flow against a local upstream and checks the
// layout jmux depends on: bare root, fetch refspec, origin/HEAD, and the
// default-branch worktree.
func TestClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	upstream := initUpstream(t)
	dest := filepath.Join(t.TempDir(), "cloned")

	wtPath, err := clone(upstream, dest)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	if repo.FindBareRoot(wtPath) != dest {
		t.Errorf("FindBareRoot(%q) = %q, want %q", wtPath, repo.FindBareRoot(wtPath), dest)
	}
	if def := gitctl.DefaultBranch(dest); def != "main" {
		t.Errorf("DefaultBranch = %q, want main", def)
	}
	if wtPath != filepath.Join(dest, "main") {
		t.Errorf("worktree at %q, want %q", wtPath, filepath.Join(dest, "main"))
	}
	if _, err := os.Stat(filepath.Join(wtPath, "README")); err != nil {
		t.Errorf("worktree not checked out: %v", err)
	}

	if _, err := clone(upstream, dest); err == nil {
		t.Error("cloning over an existing dest should error")
	}
}

func initUpstream(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "upstream")
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}
