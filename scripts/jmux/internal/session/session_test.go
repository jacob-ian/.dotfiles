package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"jmux/internal/nvimctl"
	"jmux/internal/tmuxctl"
)

// fakeBareRepo creates a directory that repo detects as a bare repo (HEAD file
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

// TestKillReapsNvimCore checks end-to-end that Kill takes down the detached
// nvim --embed core, not just the tmux session. Skips unless tmux and nvim are
// both present.
func TestKillReapsNvimCore(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim not installed")
	}

	const name = "jmux_reap_e2e"
	exec.Command("tmux", "kill-session", "-t="+name).Run()
	if err := tmuxctl.NewSession(name, "/tmp", "nvim", "nvim"); err != nil {
		t.Fatalf("new-session: %v", err)
	}
	t.Cleanup(func() { exec.Command("tmux", "kill-session", "-t="+name).Run() })

	var reap []int
	for range 40 {
		time.Sleep(250 * time.Millisecond)
		if reap = nvimctl.Processes(tmuxctl.PanePIDs(name)); len(reap) > 0 {
			break
		}
	}
	if len(reap) == 0 {
		t.Fatal("never found an nvim --embed core for the session")
	}

	Kill(name)

	for range 20 {
		time.Sleep(100 * time.Millisecond)
		if !slices.ContainsFunc(reap, alive) {
			break
		}
	}
	if left := aliveOf(reap); len(left) > 0 {
		t.Fatalf("processes still alive after Kill: %v", left)
	}
	if tmuxctl.HasSession(name) {
		t.Fatal("session still exists after Kill")
	}
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func aliveOf(pids []int) []int {
	var out []int
	for _, p := range pids {
		if alive(p) {
			out = append(out, p)
		}
	}
	return out
}
