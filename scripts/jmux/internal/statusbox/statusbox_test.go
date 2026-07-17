package statusbox

import (
	"strings"
	"testing"
	"time"
)

var (
	t1 = time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	t2 = t1.Add(time.Minute)
	t3 = t1.Add(2 * time.Minute)
)

func waiting(kind, id, label string, since time.Time) Notice {
	return Notice{Kind: kind, ID: id, Label: label, Verb: "needs input", Plural: "need input", Since: since}
}

func TestRenderNotification(t *testing.T) {
	box, disp, _ := renderBox([]Notice{waiting("claude.needs_input", "%5", "euc-web:2", t1)}, nil)
	want := "#[fg=yellow]#[range=user|jmux-content]✻ euc-web:2 needs input#[norange] │#[range=user|jmux-x] ✕ #[norange]#[fg=default]"
	if box != want {
		t.Errorf("box = %q, want %q", box, want)
	}
	if disp == nil || disp.Kind != "claude.needs_input" || disp.ID != "%5" {
		t.Errorf("display = %+v, want claude.needs_input/%%5", disp)
	}
}

func TestRenderQueuedCountAndOrder(t *testing.T) {
	notices := []Notice{
		waiting("claude.needs_input", "%2", "new:1", t2),
		waiting("claude.needs_input", "%1", "old:1", t1),
	}
	box, disp, _ := renderBox(notices, nil)
	if disp.ID != "%2" {
		t.Errorf("display id = %s, want newest %%2", disp.ID)
	}
	wantText := "✻ new:1 needs input +1"
	if !strings.Contains(box, wantText) {
		t.Errorf("box = %q, want it to contain %q", box, wantText)
	}
}

func TestDismissalRevealsNext(t *testing.T) {
	notices := []Notice{
		waiting("claude.needs_input", "%2", "new:1", t2),
		waiting("claude.needs_input", "%1", "old:1", t1),
	}
	box, disp, keep := renderBox(notices, map[string]time.Time{"claude.needs_input %2": t3})
	if disp.ID != "%1" {
		t.Errorf("display id = %s, want %%1 after dismissing %%2", disp.ID)
	}
	if strings.Contains(box, "+") {
		t.Errorf("box = %q, want no queued count", box)
	}
	if _, ok := keep["claude.needs_input %2"]; !ok {
		t.Errorf("keep = %v, want the %%2 dismissal retained", keep)
	}
}

func TestAllDismissedShowsSummary(t *testing.T) {
	notices := []Notice{
		waiting("claude.needs_input", "%1", "a:1", t1),
		waiting("claude.needs_input", "%2", "b:1", t2),
	}
	box, disp, _ := renderBox(notices, map[string]time.Time{"claude.needs_input %1": t3, "claude.needs_input %2": t3})
	want := "#[fg=colour245]✻ 2 need input #[fg=default]"
	if box != want {
		t.Errorf("box = %q, want %q", box, want)
	}
	if disp != nil {
		t.Errorf("display = %+v, want nil in summary state", disp)
	}

	box, _, _ = renderBox(notices[:1], map[string]time.Time{"claude.needs_input %1": t3})
	if !strings.Contains(box, "1 needs input") {
		t.Errorf("box = %q, want singular verb", box)
	}
}

func TestSummaryGroupsMixedVerbs(t *testing.T) {
	notices := []Notice{
		waiting("claude.needs_input", "%1", "a:1", t1),
		waiting("claude.needs_input", "%2", "b:1", t1),
		{Kind: "pr.review_requested", ID: "o/r#1", Label: "r#1", Verb: "needs review", Plural: "need review"},
	}
	dismissed := map[string]time.Time{"claude.needs_input %1": t3, "claude.needs_input %2": t3, "pr.review_requested o/r#1": t3}
	box, _, _ := renderBox(notices, dismissed)
	want := "#[fg=colour245]✻ 2 need input · 1 needs review #[fg=default]"
	if box != want {
		t.Errorf("box = %q, want %q", box, want)
	}
}

func TestRenotificationInvalidatesDismissal(t *testing.T) {
	notices := []Notice{waiting("claude.needs_input", "%1", "a:1", t3)}
	box, _, keep := renderBox(notices, map[string]time.Time{"claude.needs_input %1": t2})
	if !strings.Contains(box, "needs input") {
		t.Errorf("box = %q, want the renewed claim shown", box)
	}
	if len(keep) != 0 {
		t.Errorf("keep = %v, want the stale dismissal pruned", keep)
	}
}

func TestZeroSinceDismissalSticks(t *testing.T) {
	notices := []Notice{{Kind: "pr.review_requested", ID: "o/r#1", Label: "r#1", Verb: "needs review"}}
	box, disp, keep := renderBox(notices, map[string]time.Time{"pr.review_requested o/r#1": t1})
	if disp != nil || !strings.Contains(box, "1 needs review") {
		t.Errorf("box = %q display = %+v, want dismissed notice in summary only", box, disp)
	}
	if _, ok := keep["pr.review_requested o/r#1"]; !ok {
		t.Errorf("keep = %v, want zero-Since dismissal retained", keep)
	}
}

func TestGoneNoticesPruneDismissals(t *testing.T) {
	box, _, keep := renderBox(nil, map[string]time.Time{"claude.needs_input %1": t3})
	if box != "" {
		t.Errorf("box = %q, want empty", box)
	}
	if len(keep) != 0 {
		t.Errorf("keep = %v, want all dismissals pruned", keep)
	}
}

func TestSourceDuplicatePanics(t *testing.T) {
	Source("dup", func() []Notice { return nil })
	defer func() {
		if recover() == nil {
			t.Error("duplicate Source did not panic")
		}
	}()
	Source("dup", func() []Notice { return nil })
}

func TestHandlerDuplicatePanics(t *testing.T) {
	Handler("dup", nil)
	defer func() {
		if recover() == nil {
			t.Error("duplicate Handler did not panic")
		}
	}()
	Handler("dup", nil)
}

func TestFindMatchesByKey(t *testing.T) {
	notices := []Notice{
		{Kind: "claude.needs_input", ID: "%1", Label: "a:1"},
		{Kind: "claude.needs_input", ID: "%2", Label: "b:1"},
	}
	n, ok := find(notices, "claude.needs_input %1")
	if !ok || n.Label != "a:1" {
		t.Errorf("find(claude.needs_input %%1) = %+v %v, want the %%1 notice", n, ok)
	}
	if _, ok := find(notices, "claude.needs_input %9"); ok {
		t.Error("find(claude.needs_input %9) matched, want not found")
	}
}

func TestTruncateLabel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"short:2", "short:2"},
		{"exactly-24-runes-long:12", "exactly-24-runes-long:12"},
		{"a-very-long-session-name-indeed:3", "a-very-long-session-n…:3"},
		{"no-window-separator-and-very-long", "no-window-separator-and…"},
	}
	for _, c := range cases {
		if got := truncateLabel(c.in); got != c.want {
			t.Errorf("truncateLabel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEscapesLabel(t *testing.T) {
	box, _, _ := renderBox([]Notice{waiting("claude.needs_input", "%1", "a#b:1", t1)}, nil)
	if !strings.Contains(box, "a##b:1") {
		t.Errorf("box = %q, want '#' doubled in label", box)
	}
}
