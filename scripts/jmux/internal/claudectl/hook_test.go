package claudectl

import (
	"path/filepath"
	"testing"

	"jmux/internal/repo"
	"jmux/internal/tag"
)

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
	return badges[0].Text
}

func TestStatusTransitions(t *testing.T) {
	withCacheDir(t)
	dir := t.TempDir()

	steps := []struct {
		in   hookInput
		want string
	}{
		{hookInput{HookEventName: "UserPromptSubmit", CWD: dir}, "claude ●"},
		{hookInput{HookEventName: "Notification", NotificationType: "permission_prompt", CWD: dir}, "claude needs input"},
		{hookInput{HookEventName: "Notification", NotificationType: "auth_success", CWD: dir}, "claude needs input"},
		{hookInput{HookEventName: "UserPromptSubmit", CWD: dir}, "claude ●"},
		{hookInput{HookEventName: "Stop", CWD: dir}, "claude idle"},
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

func TestStatusIgnoresEmptyCWD(t *testing.T) {
	withCacheDir(t)
	status(hookInput{HookEventName: "Stop"})
	if n := len(tag.All()); n != 0 {
		t.Fatalf("tag store has %d entries, want 0", n)
	}
}
