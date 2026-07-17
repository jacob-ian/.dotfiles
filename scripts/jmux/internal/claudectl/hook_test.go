package claudectl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"jmux/internal/repo"
	"jmux/internal/tag"
)

// TestMain wires the claude tag renderer, as main does for the binary.
func TestMain(m *testing.M) {
	Register()
	os.Exit(m.Run())
}

// withCacheDir points os.UserCacheDir at a temp dir for the test, covering both
// the linux ($XDG_CACHE_HOME) and darwin ($HOME/Library/Caches) resolutions.
func withCacheDir(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
}

func badgeText(dir string) string {
	badges := tag.All()[repo.Resolve(dir)]
	if len(badges) == 0 {
		return ""
	}
	return badges[0].Text()
}

func TestStatusTransitions(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	steps := []struct {
		in   hookInput
		want string
	}{
		{hookInput{HookEventName: "UserPromptSubmit", CWD: dir}, "✻ working"},
		{hookInput{HookEventName: "Notification", NotificationType: "permission_prompt", CWD: dir}, "✻ needs input"},
		{hookInput{HookEventName: "Notification", NotificationType: "auth_success", CWD: dir}, "✻ needs input"},
		{hookInput{HookEventName: "UserPromptSubmit", CWD: dir}, "✻ working"},
		{hookInput{HookEventName: "Stop", CWD: dir}, "✻ idle"},
		{hookInput{HookEventName: "SessionEnd", CWD: dir}, ""},
	}
	for i, s := range steps {
		status(s.in)
		if got := badgeText(dir); got != s.want {
			t.Fatalf("step %d (%s/%s): badge = %q, want %q",
				i, s.in.HookEventName, s.in.NotificationType, got, s.want)
		}
	}
}

func TestStatusPerSession(t *testing.T) {
	withCacheDir(t)
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()

	status(hookInput{HookEventName: "UserPromptSubmit", CWD: dir, SessionID: "aaa"})
	status(hookInput{HookEventName: "UserPromptSubmit", CWD: dir, SessionID: "bbb"})
	if n := len(tag.All()[repo.Resolve(dir)]); n != 2 {
		t.Fatalf("badges = %d, want one per session (2)", n)
	}

	status(hookInput{HookEventName: "Stop", CWD: dir, SessionID: "bbb"})
	badges := tag.All()[repo.Resolve(dir)]
	if badges[0].Text() != "✻ working" || badges[1].Text() != "✻ idle" {
		t.Fatalf("badges = %q, %q; want session aaa still working, bbb idle",
			badges[0].Text(), badges[1].Text())
	}

	status(hookInput{HookEventName: "SessionEnd", CWD: dir, SessionID: "bbb"})
	badges = tag.All()[repo.Resolve(dir)]
	if len(badges) != 1 || badges[0].Text() != "✻ working" {
		t.Fatalf("badges after bbb SessionEnd = %v, want only aaa working", badges)
	}
}

func TestStatusIgnoresEmptyCWD(t *testing.T) {
	withCacheDir(t)
	status(hookInput{HookEventName: "Stop"})
	if n := len(tag.All()); n != 0 {
		t.Fatalf("tag store has %d entries, want 0", n)
	}
}

var (
	tA = time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	tB = tA.Add(time.Minute)
)

func waitingTag(pane string, at time.Time) tag.Tag {
	return tag.New(tagKind, pane, tagData{Status: statusNeedsInput, UpdatedAt: at})
}

func workingTag(pane string, at time.Time) tag.Tag {
	return tag.New(tagKind, pane, tagData{Status: statusWorking, UpdatedAt: at})
}

func TestNoticesFromTags(t *testing.T) {
	panes := map[string]string{"%1": "a:1", "%2": "b:2"}

	got := noticesFromTags(map[string][]tag.Tag{"/ws/euc-web": {waitingTag("%1", tA)}}, panes)
	if len(got) != 1 || got[0].ID != "%1" || got[0].Label != "euc-web" ||
		got[0].Verb != "needs input" || !got[0].Since.Equal(tA) {
		t.Errorf("notices = %+v, want one for %%1 labelled euc-web", got)
	}

	// Paneless, dead-pane, and non-claude tags never notice.
	got = noticesFromTags(map[string][]tag.Tag{"/w": {
		waitingTag("", tA),
		waitingTag("%9", tA),
		tag.New("pr", "%1", tagData{Status: statusNeedsInput, UpdatedAt: tA}),
	}}, panes)
	if len(got) != 0 {
		t.Errorf("notices = %+v, want none", got)
	}

	// A newer quiet session in the same pane supersedes a stale claim.
	got = noticesFromTags(map[string][]tag.Tag{"/w": {
		waitingTag("%1", tA), workingTag("%1", tB),
	}}, panes)
	if len(got) != 0 {
		t.Errorf("notices = %+v, want the stale claim superseded", got)
	}

	// And the reverse: a newer claim wins over an older quiet session.
	got = noticesFromTags(map[string][]tag.Tag{"/w": {
		workingTag("%2", tA), waitingTag("%2", tB),
	}}, panes)
	if len(got) != 1 || got[0].ID != "%2" {
		t.Errorf("notices = %+v, want one for %%2", got)
	}
}
