package claudectl

import (
	"testing"
	"time"

	"jmux/internal/tag"
)

func TestNewestStatusPicksLiveLatest(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tags := []tag.Tag{
		tag.New("claude", "%1", tagData{Status: "idle", UpdatedAt: old}),
		tag.New("claude", "%2", tagData{Status: "working", UpdatedAt: old.Add(time.Minute)}),
		tag.New("claude", "%3", tagData{Status: "needs_input", UpdatedAt: old.Add(time.Hour)}),
		tag.New("pr", "%2", map[string]string{"n": "1"}),
	}
	panes := map[string]string{"%1": "1", "%2": "2"}
	if got := newestStatus(tags, panes); got != "working" {
		t.Fatalf("got %q, want working (dead pane %%3 skipped)", got)
	}
	if got := newestStatus(tags, map[string]string{}); got != "" {
		t.Fatalf("no live panes: got %q", got)
	}
}
