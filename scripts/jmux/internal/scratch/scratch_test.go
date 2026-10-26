package scratch_test

import (
	"testing"

	"jmux/internal/scratch"
)

func TestSessionName(t *testing.T) {
	const today = "2026-10-26"
	tests := map[string]struct {
		query string
		taken []string
		want  string
	}{
		"empty is today":           {want: today},
		"name is suffixed":         {query: "dmd names", want: "2026-10-26-dmd-names"},
		"dated query kept":         {query: "2026-10-20-foo", want: "2026-10-20-foo"},
		"bare date kept":           {query: "2026-10-20", want: "2026-10-20"},
		"whitespace and case":      {query: "  Foo  Bar ", want: "2026-10-26-foo-bar"},
		"collision bumps":          {taken: []string{today}, want: today + "-2"},
		"repeated collision":       {query: "x", taken: []string{today + "-x", today + "-x-2"}, want: today + "-x-3"},
		"date-like but not a date": {query: "2026-1-2", want: today + "-2026-1-2"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			taken := func(n string) bool {
				for _, x := range tt.taken {
					if x == n {
						return true
					}
				}
				return false
			}
			if got := scratch.SessionName(tt.query, today, taken); got != tt.want {
				t.Errorf("SessionName(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}
