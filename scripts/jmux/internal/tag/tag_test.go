package tag

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"jmux/internal/cachefile"
	"jmux/internal/repo"
)

// The tests register their own kind so they exercise the store and registry
// without depending on any real producer's presentation.
func init() {
	Register("test", func(data json.RawMessage) (string, Color) {
		var d struct {
			Label string `json:"label"`
		}
		if json.Unmarshal(data, &d) != nil || d.Label == "" {
			return "", ""
		}
		return d.Label, Green
	})
}

func testTag(label string) Tag {
	return New("test", "", map[string]string{"label": label})
}

// withCacheDir points os.UserCacheDir at a temp dir for the test, covering both
// the linux ($XDG_CACHE_HOME) and darwin ($HOME/Library/Caches) resolutions.
func withCacheDir(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
}

func TestSetAllRoundTrips(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	Set(dir, "test", testTag("first"))

	tags := All()[repo.Resolve(dir)]
	if len(tags) != 1 || tags[0].Text() != "first" {
		t.Fatalf("All()[%q] = %+v, want one tag rendering %q", dir, tags, "first")
	}
}

func TestSetReplacesSameNamespace(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	Set(dir, "test", testTag("first"))
	Set(dir, "test", testTag("second"))

	tags := All()[repo.Resolve(dir)]
	if len(tags) != 1 || tags[0].Text() != "second" {
		t.Fatalf("All()[%q] = %+v, want one tag rendering %q", dir, tags, "second")
	}
}

func TestAllOrdersByNamespace(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	Set(dir, "zeta", testTag("Z"))
	Set(dir, "alpha", testTag("A"))

	tags := All()[repo.Resolve(dir)]
	if len(tags) != 2 || tags[0].Text() != "A" || tags[1].Text() != "Z" {
		t.Fatalf("All()[%q] = %+v, want [A Z] (namespace order)", dir, tags)
	}
}

func TestSetPrunesMissingDirs(t *testing.T) {
	withCacheDir(t)
	live := t.TempDir()
	gone := filepath.Join(t.TempDir(), "removed-worktree") // never created

	Set(gone, "test", testTag("stale")) // tags a path that doesn't exist on disk
	Set(live, "test", testTag("live"))  // this write prunes the missing entry

	all := All()
	if _, ok := all[repo.Resolve(gone)]; ok {
		t.Errorf("expected stale entry for %q to be pruned", gone)
	}
	if tags := all[repo.Resolve(live)]; len(tags) != 1 || tags[0].Text() != "live" {
		t.Errorf("All()[%q] = %+v, want one tag rendering %q", live, tags, "live")
	}
}

func TestLegacyEntriesAreSkippedAndPruned(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	// a pre-semantic store entry: rendered text, no kind.
	legacy := map[string]map[string]map[string]string{
		repo.Resolve(dir): {"claude": {"text": "✻ working", "color": "green"}},
	}
	cachefile.Write(storeFile, legacy)

	if n := len(All()); n != 0 {
		t.Fatalf("All() shows %d paths for a legacy-only store, want 0", n)
	}

	// any write prunes the kindless entry from disk.
	Set(dir, "test", testTag("fresh"))
	s := store{}
	cachefile.Read(storeFile, &s)
	if _, ok := s[repo.Resolve(dir)]["claude"]; ok {
		t.Fatal("legacy kindless entry survived a Set")
	}
}

func TestAllSkipsUnregisteredKinds(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	Set(dir, "future", New("future", "", map[string]int{"v": 1}))
	Set(dir, "test", testTag("shown"))

	tags := All()[repo.Resolve(dir)]
	if len(tags) != 1 || tags[0].Text() != "shown" {
		t.Fatalf("All()[%q] = %+v, want only the registered kind", dir, tags)
	}
}

func TestUnsetPrefix(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	Set(dir, "test:abc", testTag("one"))
	Set(dir, "test:def", testTag("two"))
	Set(dir, "other", testTag("kept"))

	UnsetPrefix(dir, "test")

	tags := All()[repo.Resolve(dir)]
	if len(tags) != 1 || tags[0].Text() != "kept" {
		t.Fatalf("tags = %+v, want only the other-namespace tag", tags)
	}
}

func TestUnsetPrefixDropsEmptyPath(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	Set(dir, "test:abc", testTag("one"))
	UnsetPrefix(dir, "test")

	if n := len(All()); n != 0 {
		t.Fatalf("store has %d paths, want 0", n)
	}
}

func TestAllNoStore(t *testing.T) {
	withCacheDir(t)
	if got := All(); len(got) != 0 {
		t.Errorf("All() with no store = %v, want empty", got)
	}
}

func TestRender(t *testing.T) {
	if got := testTag("x").Render(); got != "\x1b[32mx\x1b[0m" {
		t.Errorf("Render() = %q, want green-wrapped x", got)
	}
	if got := (Tag{}).Render(); got != "" {
		t.Errorf("kindless Render() = %q, want empty", got)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate Register did not panic")
		}
	}()
	Register("test", func(json.RawMessage) (string, Color) { return "", "" })
}
