package tmuxctl_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"jmux/internal/tmuxctl"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/usr/bin/jmux":        `'/usr/bin/jmux'`,
		"/path/with a space/x": `'/path/with a space/x'`,
		"it's":                 `'it'\''s'`,
		"two\nlines":           "'two\nlines'",
	}
	for in, want := range cases {
		if got := tmuxctl.ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

// A private tmux server: own socket under TMUX_TMPDIR, TMUX cleared so the
// client doesn't follow the server this test runs inside.
func TestSessionDirs(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("TMUX", "")
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
			t.Fatalf("tmux %v: %v\n%s", args, err, out)
		}
	}
	stamped := t.TempDir()
	// tmux reports pane paths symlink-resolved (/var -> /private/var on macOS).
	unstamped, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run("new-session", "-d", "-s", "stamped", "-c", stamped)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	run("set-option", "-t", "stamped", "@jmux_dir", "/stamped/elsewhere")
	run("new-session", "-d", "-s", "unstamped", "-c", unstamped)

	got := tmuxctl.SessionDirs()
	slices.Sort(got)
	want := []string{"/stamped/elsewhere", unstamped}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("SessionDirs() = %q, want %q", got, want)
	}
}
