package tmuxctl_test

import (
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
