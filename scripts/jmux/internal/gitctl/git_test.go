package gitctl

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
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

func TestDefaultBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	tests := map[string]struct {
		setup func(bare string)
		want  string
	}{
		"origin/HEAD wins": {
			setup: func(bare string) {
				run(bare, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
			},
			want: "main",
		},
		"no remote falls back to HEAD": {want: "history"},
		"detached HEAD without remote": {
			setup: func(bare string) {
				if err := os.WriteFile(bare+"/HEAD", []byte("0123456789abcdef0123456789abcdef01234567\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			bare := t.TempDir()
			run(bare, "init", "-q", "--bare")
			run(bare, "symbolic-ref", "HEAD", "refs/heads/history")
			if tt.setup != nil {
				tt.setup(bare)
			}
			if got := DefaultBranch(bare); got != tt.want {
				t.Errorf("DefaultBranch() = %q, want %q", got, tt.want)
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

func TestWorktreeForBranch(t *testing.T) {
	const porcelain = `worktree /repos/app/main
HEAD aaa111
branch refs/heads/main

worktree /repos/app/feature-x
HEAD bbb222
branch refs/heads/feature/x

worktree /repos/app/wip
HEAD ccc333
detached
`
	tests := []struct {
		name   string
		branch string
		want   string
	}{
		{"first entry", "main", "/repos/app/main"},
		{"branch with slash at non-conventional path", "feature/x", "/repos/app/feature-x"},
		{"detached entry has no branch", "ccc333", ""},
		{"not checked out anywhere", "ghost", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := worktreeForBranch(porcelain, tt.branch); got != tt.want {
				t.Errorf("worktreeForBranch(%q) = %q, want %q", tt.branch, got, tt.want)
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

// divergedRepo builds a repo whose feature branch has two commits main lacks
// and main one commit feature lacks, returning its path.
func divergedRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "base")
	run("checkout", "-q", "-b", "feature")
	run("commit", "-q", "--allow-empty", "-m", "feature one")
	run("commit", "-q", "--allow-empty", "-m", "feature two")
	run("checkout", "-q", "main")
	run("commit", "-q", "--allow-empty", "-m", "main only")
	run("checkout", "-q", "feature")
	return dir
}

func TestAheadBehind(t *testing.T) {
	dir := divergedRepo(t)
	tests := map[string]struct {
		ref           string
		ahead, behind int
		ok            bool
	}{
		"diverged from main": {ref: "main", ahead: 2, behind: 1, ok: true},
		"same commit":        {ref: "feature", ok: true},
		"unknown ref":        {ref: "origin/main"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ahead, behind, ok := AheadBehind(dir, tt.ref)
			if ahead != tt.ahead || behind != tt.behind || ok != tt.ok {
				t.Errorf("AheadBehind(%q) = (%d, %d, %v), want (%d, %d, %v)", tt.ref, ahead, behind, ok, tt.ahead, tt.behind, tt.ok)
			}
		})
	}
}

func TestLogOneline(t *testing.T) {
	dir := divergedRepo(t)
	tests := map[string]struct {
		revRange string
		n        int
		want     []string
	}{
		"unique commits newest first": {revRange: "main..HEAD", n: 10, want: []string{"feature two", "feature one"}},
		"limit applies":               {revRange: "main..HEAD", n: 1, want: []string{"feature two"}},
		"nothing unique":              {revRange: "feature..HEAD", n: 10},
		"unknown ref is empty":        {revRange: "origin/main..HEAD", n: 10},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			out := LogOneline(dir, tt.revRange, tt.n)
			var got []string
			for line := range strings.SplitSeq(out, "\n") {
				if _, subject, ok := strings.Cut(line, " "); ok {
					got = append(got, subject)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("LogOneline(%q, %d) subjects = %q, want %q", tt.revRange, tt.n, got, tt.want)
			}
		})
	}
}
