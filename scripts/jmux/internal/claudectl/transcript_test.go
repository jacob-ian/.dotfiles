package claudectl

import (
	"strings"
	"testing"
)

func TestTailKeepsVisibleTurnsOnly(t *testing.T) {
	lines := []string{
		`{"type":"user","timestamp":"2026-11-12T03:39:04Z","message":{"content":"fix the thing"}}`,
		`{"type":"assistant","timestamp":"2026-11-12T03:39:05Z","message":{"content":[{"type":"thinking","thinking":"hmm"}]}}`,
		`{"type":"assistant","timestamp":"2026-11-12T03:39:06Z","message":{"content":[{"type":"tool_use","name":"Bash","input":{}}]}}`,
		`{"type":"user","timestamp":"2026-11-12T03:39:07Z","message":{"content":[{"type":"tool_result","content":"ok"}]}}`,
		`{"type":"user","isMeta":true,"timestamp":"2026-11-12T03:39:07Z","message":{"content":[{"type":"text","text":"skill body"}]}}`,
		`{"type":"assistant","timestamp":"2026-11-12T03:39:08Z","message":{"content":[{"type":"text","text":"Looking now."}]}}`,
		`{"type":"assistant","timestamp":"2026-11-12T03:39:09Z","message":{"content":[{"type":"text","text":"Done: fixed."}]}}`,
		`{"type":"api-request-blob","message":{"content":[{"type":"text","text":"noise"}]}}`,
		`{"type":"user","timestamp":"2026-11-12T03:40:00Z","message":{"content":[{"type":"text","text":"thanks, now push"}]}}`,
		`not json`,
	}
	msgs, err := Tail(strings.NewReader(strings.Join(lines, "\n")+"\n"), 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []Message{
		{Role: "user", Text: "fix the thing"},
		{Role: "assistant", Text: "Looking now.\n\nDone: fixed."},
		{Role: "user", Text: "thanks, now push"},
	}
	if len(msgs) != len(want) {
		t.Fatalf("got %d messages, want %d: %+v", len(msgs), len(want), msgs)
	}
	for i := range want {
		if msgs[i].Role != want[i].Role || msgs[i].Text != want[i].Text {
			t.Errorf("message %d: got %+v, want %+v", i, msgs[i], want[i])
		}
		if msgs[i].At.IsZero() {
			t.Errorf("message %d has no timestamp", i)
		}
	}
	last, _ := Tail(strings.NewReader(strings.Join(lines, "\n")), 1)
	if len(last) != 1 || last[0].Text != "thanks, now push" {
		t.Fatalf("n=1: got %+v", last)
	}
}

func TestProjectSlug(t *testing.T) {
	if got := projectSlug("/Users/jacob.matthews/.config"); got != "-Users-jacob-matthews--config" {
		t.Fatalf("got %q", got)
	}
}
