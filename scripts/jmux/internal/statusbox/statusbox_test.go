package statusbox

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"jmux/internal/tag"
)

type testData struct {
	Waiting bool      `json:"waiting"`
	Since   time.Time `json:"since"`
}

func TestMain(m *testing.M) {
	tag.RegisterAttention("test", func(data json.RawMessage) tag.Attention {
		var d testData
		if json.Unmarshal(data, &d) != nil {
			return tag.Attention{}
		}
		a := tag.Attention{Since: d.Since}
		if d.Waiting {
			a.Verb = "needs input"
		}
		return a
	})
	os.Exit(m.Run())
}

var (
	t1 = time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	t2 = t1.Add(time.Minute)
	t3 = t1.Add(2 * time.Minute)
)

func waiting(pane string, since time.Time) tag.Tag {
	return tag.New("test", pane, testData{Waiting: true, Since: since})
}

func quiet(pane string, since time.Time) tag.Tag {
	return tag.New("test", pane, testData{Since: since})
}

func TestRenderNotification(t *testing.T) {
	items := buildItems(
		map[string][]tag.Tag{"/w": {waiting("%5", t1)}},
		map[string]string{"%5": "euc-web:2"},
	)
	box, disp, _ := renderBox(items, nil)
	want := "#[fg=yellow]#[range=user|jmux-go]✻ euc-web:2 needs input#[norange] │#[range=user|jmux-x] ✕ #[norange]#[fg=default]"
	if box != want {
		t.Errorf("box = %q, want %q", box, want)
	}
	if disp == nil || disp.Kind != "test" || disp.Pane != "%5" {
		t.Errorf("display = %+v, want test/%%5", disp)
	}
}

func TestRenderQueuedCountAndOrder(t *testing.T) {
	items := buildItems(
		map[string][]tag.Tag{"/a": {waiting("%1", t1)}, "/b": {waiting("%2", t2)}},
		map[string]string{"%1": "old:1", "%2": "new:1"},
	)
	box, disp, _ := renderBox(items, nil)
	if disp.Pane != "%2" {
		t.Errorf("display pane = %s, want newest %%2", disp.Pane)
	}
	wantText := "✻ new:1 needs input +1"
	if !strings.Contains(box, wantText) {
		t.Errorf("box = %q, want it to contain %q", box, wantText)
	}
}

func TestDismissalRevealsNext(t *testing.T) {
	items := buildItems(
		map[string][]tag.Tag{"/a": {waiting("%1", t1)}, "/b": {waiting("%2", t2)}},
		map[string]string{"%1": "old:1", "%2": "new:1"},
	)
	box, disp, keep := renderBox(items, map[string]time.Time{"test %2": t3})
	if disp.Pane != "%1" {
		t.Errorf("display pane = %s, want %%1 after dismissing %%2", disp.Pane)
	}
	if strings.Contains(box, "+") {
		t.Errorf("box = %q, want no queued count", box)
	}
	if _, ok := keep["test %2"]; !ok {
		t.Errorf("keep = %v, want the %%2 dismissal retained", keep)
	}
}

func TestAllDismissedShowsSummary(t *testing.T) {
	items := buildItems(
		map[string][]tag.Tag{"/a": {waiting("%1", t1)}, "/b": {waiting("%2", t2)}},
		map[string]string{"%1": "a:1", "%2": "b:1"},
	)
	box, disp, _ := renderBox(items, map[string]time.Time{"test %1": t3, "test %2": t3})
	want := "#[fg=colour245]✻ 2 waiting #[fg=default]"
	if box != want {
		t.Errorf("box = %q, want %q", box, want)
	}
	if disp != nil {
		t.Errorf("display = %+v, want nil in summary state", disp)
	}
}

func TestRenotificationInvalidatesDismissal(t *testing.T) {
	items := buildItems(
		map[string][]tag.Tag{"/a": {waiting("%1", t3)}},
		map[string]string{"%1": "a:1"},
	)
	box, _, keep := renderBox(items, map[string]time.Time{"test %1": t2})
	if !strings.Contains(box, "needs input") {
		t.Errorf("box = %q, want the renewed claim shown", box)
	}
	if len(keep) != 0 {
		t.Errorf("keep = %v, want the stale dismissal pruned", keep)
	}
}

func TestQuietedAndDeadClaimsPruneDismissals(t *testing.T) {
	// %1's claim was quieted by a newer tag in the same pane; %2's pane died.
	items := buildItems(
		map[string][]tag.Tag{"/a": {waiting("%1", t1), quiet("%1", t2), waiting("%2", t1)}},
		map[string]string{"%1": "a:1"},
	)
	if len(items) != 0 {
		t.Fatalf("items = %v, want none", items)
	}
	box, _, keep := renderBox(items, map[string]time.Time{"test %1": t3, "test %2": t3})
	if box != "" {
		t.Errorf("box = %q, want empty", box)
	}
	if len(keep) != 0 {
		t.Errorf("keep = %v, want all dismissals pruned", keep)
	}
}

func TestIgnoresPanelessAndUnregistered(t *testing.T) {
	items := buildItems(
		map[string][]tag.Tag{"/a": {
			waiting("", t1),
			tag.New("unregistered", "%1", testData{Waiting: true, Since: t1}),
		}},
		map[string]string{"%1": "a:1"},
	)
	if len(items) != 0 {
		t.Errorf("items = %v, want none", items)
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
	items := buildItems(
		map[string][]tag.Tag{"/a": {waiting("%1", t1)}},
		map[string]string{"%1": "a#b:1"},
	)
	box, _, _ := renderBox(items, nil)
	if !strings.Contains(box, "a##b:1") {
		t.Errorf("box = %q, want '#' doubled in label", box)
	}
}
