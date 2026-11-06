package pr

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"jmux/internal/gitctl"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func syncRepos(t *testing.T) (origin, local string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	origin = filepath.Join(t.TempDir(), "origin")
	local = filepath.Join(t.TempDir(), "local")
	gitIn(t, ".", "init", "-q", "-b", "feature", origin)
	os.WriteFile(filepath.Join(origin, "f"), []byte("one\n"), 0o644)
	gitIn(t, origin, "add", "f")
	gitIn(t, origin, "commit", "-q", "-m", "one")
	gitIn(t, ".", "clone", "-q", origin, local)
	return origin, local
}

func head(t *testing.T, dir string) string {
	t.Helper()
	h, ok := gitctl.RevParse(dir, "HEAD")
	if !ok {
		t.Fatalf("no HEAD in %s", dir)
	}
	return h
}

func TestSync(t *testing.T) {
	tests := map[string]struct {
		arrange func(t *testing.T, origin, local string)
		want    Outcome
		moved   bool // HEAD ends at origin's head
		reason  string
	}{
		"up to date": {
			arrange: func(*testing.T, string, string) {},
			want:    Current, moved: true,
		},
		"fast-forward": {
			arrange: func(t *testing.T, origin, _ string) {
				gitIn(t, origin, "commit", "-q", "--allow-empty", "-m", "two")
			},
			want: FastForwarded, moved: true,
		},
		"rewritten, nothing local": {
			arrange: func(t *testing.T, origin, _ string) {
				gitIn(t, origin, "commit", "-q", "--amend", "-m", "one amended")
			},
			want: Reset, moved: true,
		},
		"rewritten, local commit kept": {
			arrange: func(t *testing.T, origin, local string) {
				gitIn(t, local, "commit", "-q", "--allow-empty", "-m", "mine")
				gitIn(t, origin, "commit", "-q", "--amend", "-m", "one amended")
			},
			want: Diverged, reason: reasonCommits,
		},
		"rewritten, dirty tree kept": {
			arrange: func(t *testing.T, origin, local string) {
				os.WriteFile(filepath.Join(local, "f"), []byte("edited\n"), 0o644)
				gitIn(t, origin, "commit", "-q", "--amend", "-m", "one amended")
			},
			want: Diverged, reason: reasonDirty,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			origin, local := syncRepos(t)
			tt.arrange(t, origin, local)
			before := head(t, local)

			res, err := Sync(local, local, "feature")
			if err != nil {
				t.Fatalf("Sync: %v", err)
			}
			if res.Outcome != tt.want || res.Reason != tt.reason {
				t.Errorf("Sync = %+v, want outcome %s reason %q", res, tt.want, tt.reason)
			}
			got, want := head(t, local), before
			if tt.moved {
				want = head(t, origin)
			}
			if got != want {
				t.Errorf("HEAD = %s, want %s", got, want)
			}
		})
	}
}

func TestSyncKeepsDirtyFile(t *testing.T) {
	origin, local := syncRepos(t)
	os.WriteFile(filepath.Join(local, "f"), []byte("edited\n"), 0o644)
	gitIn(t, origin, "commit", "-q", "--amend", "-m", "one amended")
	if _, err := Sync(local, local, "feature"); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(local, "f"))
	if string(data) != "edited\n" {
		t.Errorf("f = %q, want the local edit kept", data)
	}
}
