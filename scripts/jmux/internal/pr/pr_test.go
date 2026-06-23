package pr

import "testing"

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

func TestFormatRowRoundTrips(t *testing.T) {
	line := formatRow("eucalyptusvc/mobile", 42, true, "Add widget", "octocat")
	slug, num, ok := parseRepoNumber(line)
	if !ok || slug != "eucalyptusvc/mobile" || num != 42 {
		t.Fatalf("parseRepoNumber(formatRow(...)) = (%q, %d, %v); line=%q", slug, num, ok, line)
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

func TestAllowedOrgs(t *testing.T) {
	cases := []struct {
		env  string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"eucalyptusvc", []string{"eucalyptusvc"}},
		{"eucalyptusvc,getnetfluence", []string{"eucalyptusvc", "getnetfluence"}},
		{" eucalyptusvc , getnetfluence ", []string{"eucalyptusvc", "getnetfluence"}},
		{"eucalyptusvc,,", []string{"eucalyptusvc"}},
	}
	for _, c := range cases {
		t.Setenv(orgEnv, c.env)
		got := allowedOrgs()
		if len(got) != len(c.want) {
			t.Errorf("allowedOrgs() with %q = %v, want %v", c.env, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("allowedOrgs() with %q = %v, want %v", c.env, got, c.want)
				break
			}
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
