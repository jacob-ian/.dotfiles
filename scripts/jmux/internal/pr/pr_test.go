package pr

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"jmux/internal/repo"
	"jmux/internal/tag"
)

func TestMain(m *testing.M) {
	Register()
	os.Exit(m.Run())
}

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
	for _, marker := range []string{"", "  ", repo.AgentMarker} {
		line := formatRow(marker, "eucalyptusvc/mobile", 42, true, "Add widget", "octocat")
		slug, num, ok := parseRepoNumber(line)
		if !ok || slug != "eucalyptusvc/mobile" || num != 42 {
			t.Fatalf("parseRepoNumber(formatRow(%q, ...)) = (%q, %d, %v); line=%q", marker, slug, num, ok, line)
		}
	}
}

func TestRowMarker(t *testing.T) {
	if got := rowMarker("g3rtrude"); got != repo.AgentMarker {
		t.Errorf("rowMarker(g3rtrude) = %q, want the agent marker", got)
	}
	if got := rowMarker("octocat"); got != "  " {
		t.Errorf("rowMarker(octocat) = %q, want padding", got)
	}
}

func TestByline(t *testing.T) {
	tests := map[string]struct {
		author, owner, want string
	}{
		"human author":         {author: "octocat", want: "octocat"},
		"bot with owner":       {author: "g3rtrude", owner: "octocat", want: "octocat via g3rtrude"},
		"bot nobody picked up": {author: "g3rtrude", want: "g3rtrude"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := byline(tt.author, tt.owner); got != tt.want {
				t.Errorf("byline(%q, %q) = %q, want %q", tt.author, tt.owner, got, tt.want)
			}
		})
	}
}

func TestFormatItemsRowRoundTrips(t *testing.T) {
	line := formatItemsRow("  ", "eucalyptusvc/mobile", 42, true, "Add widget", "octocat", "feat/widget", "release/1.2")
	if slug, num, ok := parseRepoNumber(line); !ok || slug != "eucalyptusvc/mobile" || num != 42 {
		t.Fatalf("parseRepoNumber(formatItemsRow(...)) = (%q, %d, %v); line=%q", slug, num, ok, line)
	}
	if got := rowHeadRef(line); got != "feat/widget" {
		t.Errorf("rowHeadRef = %q, want %q", got, "feat/widget")
	}
	if got := rowBaseRef(line); got != "release/1.2" {
		t.Errorf("rowBaseRef = %q, want %q", got, "release/1.2")
	}
}

func TestRowRefsTolerateMissingBaseField(t *testing.T) {
	line := formatRow("  ", "owner/repo", 7, false, "Title", "author") + "\tfeat/x"
	if got := rowHeadRef(line); got != "feat/x" {
		t.Errorf("rowHeadRef = %q, want %q", got, "feat/x")
	}
	if got := rowBaseRef(line); got != "" {
		t.Errorf("rowBaseRef = %q, want %q", got, "")
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

func TestTagRenders(t *testing.T) {
	got := tag.New(tagKind, "", tagData{Number: 42}).Text()
	if got != "\uf407 #42" {
		t.Errorf("pr tag Text() = %q, want the glyph and #42", got)
	}
}

func TestFindLocalRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	bare := func(name, originURL string) string {
		t.Helper()
		dir := filepath.Join(root, name)
		for _, args := range [][]string{
			{"init", "-q", "--bare", dir},
			{"-C", dir, "remote", "add", "origin", originURL},
		} {
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		return dir
	}
	named := bare("widgets", "git@github.com:acme/widgets.git")
	renamed := bare("legacy-clone", "https://github.com/acme/gadgets.git")
	bare("decoy", "git@github.com:other/widgets.git")
	cfg := repo.Config{ScanRoots: []string{root}}

	tests := map[string]struct {
		slug string
		want string
	}{
		"clone named after the repo": {slug: "acme/widgets", want: named},
		"differently named clone":    {slug: "acme/gadgets", want: renamed},
		"slug case is ignored":       {slug: "ACME/Widgets", want: named},
		"no local clone":             {slug: "acme/nothing"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := findLocalRepo(cfg, tt.slug); got != tt.want {
				t.Errorf("findLocalRepo(%q) = %q, want %q", tt.slug, got, tt.want)
			}
		})
	}
}
