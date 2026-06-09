package pr

import (
	"testing"

	"jmux/internal/ghctl"
)

func TestParseNumber(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"#12  Fix the thing  ·  octocat", 12, true},
		{"#7  [draft] WIP  ·  alice", 7, true},
		{"123", 123, true},
		{"#456", 456, true},
		{"  #99 trailing", 99, true},
		{"no number here", 0, false},
		{"", 0, false},
		{"#", 0, false},
	}
	for _, c := range cases {
		got, ok := ParseNumber(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseNumber(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestFormatLineRoundTrips(t *testing.T) {
	p := ghctl.PR{Number: 42, Title: "Add widget", IsDraft: true}
	p.Author.Login = "octocat"
	line := formatLine(p)
	got, ok := ParseNumber(line)
	if !ok || got != 42 {
		t.Fatalf("ParseNumber(formatLine(...)) = (%d, %v), want (42, true); line=%q", got, ok, line)
	}
}

func TestParseRepoNumber(t *testing.T) {
	cases := []struct {
		in       string
		wantSlug string
		wantNum  int
		ok       bool
	}{
		{"eucalyptusvc/mobile#5639  dynamic visuals  ·  azizmehedi", "eucalyptusvc/mobile", 5639, true},
		{"getnetfluence/dashboard-service#225  [draft] Thing  ·  jacob-ian", "getnetfluence/dashboard-service", 225, true},
		{"owner/repo#1", "owner/repo", 1, true},
		{"#12 no slug", "", 0, false},
		{"no number here", "", 0, false},
		{"", "", 0, false},
	}
	for _, c := range cases {
		slug, num, ok := parseRepoNumber(c.in)
		if slug != c.wantSlug || num != c.wantNum || ok != c.ok {
			t.Errorf("parseRepoNumber(%q) = (%q, %d, %v), want (%q, %d, %v)", c.in, slug, num, ok, c.wantSlug, c.wantNum, c.ok)
		}
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/usr/bin/jmux":        `'/usr/bin/jmux'`,
		"/path/with a space/x": `'/path/with a space/x'`,
		"it's":                 `'it'\''s'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
