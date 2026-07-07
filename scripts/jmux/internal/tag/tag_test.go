package tag

import (
	"path/filepath"
	"testing"

	"jmux/internal/repo"
)

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

	Set(dir, "pr", Badge{Text: "PR #42", Color: Cyan})

	badges := All()[repo.Resolve(dir)]
	if len(badges) != 1 || badges[0].Text != "PR #42" || badges[0].Color != Cyan {
		t.Fatalf("All()[%q] = %+v, want one {PR #42, cyan}", dir, badges)
	}
}

func TestSetReplacesSameNamespace(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	Set(dir, "pr", Badge{Text: "PR #1"})
	Set(dir, "pr", Badge{Text: "PR #2"})

	badges := All()[repo.Resolve(dir)]
	if len(badges) != 1 || badges[0].Text != "PR #2" {
		t.Fatalf("All()[%q] = %+v, want one {PR #2}", dir, badges)
	}
}

func TestAllOrdersByNamespace(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	Set(dir, "zeta", Badge{Text: "Z"})
	Set(dir, "alpha", Badge{Text: "A"})

	badges := All()[repo.Resolve(dir)]
	if len(badges) != 2 || badges[0].Text != "A" || badges[1].Text != "Z" {
		t.Fatalf("All()[%q] = %+v, want [A Z] (namespace order)", dir, badges)
	}
}

func TestSetPrunesMissingDirs(t *testing.T) {
	withCacheDir(t)
	live := t.TempDir()
	gone := filepath.Join(t.TempDir(), "removed-worktree") // never created

	Set(gone, "pr", Badge{Text: "PR #7"}) // tags a path that doesn't exist on disk
	Set(live, "pr", Badge{Text: "PR #9"}) // this write prunes the missing entry

	all := All()
	if _, ok := all[repo.Resolve(gone)]; ok {
		t.Errorf("expected stale entry for %q to be pruned", gone)
	}
	if badges := all[repo.Resolve(live)]; len(badges) != 1 || badges[0].Text != "PR #9" {
		t.Errorf("All()[%q] = %+v, want one {PR #9}", live, badges)
	}
}

func TestUnsetPrefix(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	Set(dir, "claude", Badge{Text: "legacy"})
	Set(dir, "claude:abc", Badge{Text: "working"})
	Set(dir, "pr", Badge{Text: "open"})

	UnsetPrefix(dir, "claude")

	badges := All()[repo.Resolve(dir)]
	if len(badges) != 1 || badges[0].Text != "open" {
		t.Fatalf("badges = %+v, want only the pr badge", badges)
	}
}

func TestUnsetPrefixDropsEmptyPath(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	Set(dir, "claude:abc", Badge{Text: "working"})
	UnsetPrefix(dir, "claude")

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

func TestBadgeRender(t *testing.T) {
	if got := (Badge{Text: "x", Color: Green}).Render(); got != "\x1b[32mx\x1b[0m" {
		t.Errorf("green Render() = %q", got)
	}
	if got := (Badge{Text: "x"}).Render(); got != "x" {
		t.Errorf("default Render() = %q, want plain", got)
	}
	if got := (Badge{Text: "x", Color: Color("bogus")}).Render(); got != "x" {
		t.Errorf("unknown-colour Render() = %q, want plain", got)
	}
}
