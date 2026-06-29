package spinner

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestRenderCenters captures a frame (stderr is a pipe, so term size falls back
// to 80x24) and checks the logo block and the action line are both present and
// that every logo line starts at the same column — i.e. the block is shifted as
// a whole and keeps its shape.
func TestRenderCenters(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	render(0, "opening PR #42…")
	w.Close()
	os.Stderr = old

	raw, _ := io.ReadAll(r)
	out := strings.NewReplacer("\x1b[2J\x1b[H", "", cyan, "", reset, "").Replace(string(raw))

	if !strings.Contains(out, "opening PR #42…") {
		t.Errorf("action line missing from frame:\n%s", out)
	}

	// Each logo line still carries its own leading spaces, so it's the line's
	// content (trimmed right only) that should land at one shared column.
	left := -1
	for i, l := range logo {
		content := strings.TrimRight(l, " ")
		row := firstLineWith(out, content)
		if row == "" {
			t.Fatalf("logo line %d missing from frame:\n%s", i, out)
		}
		col := strings.Index(row, content)
		if strings.Trim(row[:col], " ") != "" {
			t.Errorf("logo line %d has non-space prefix: %q", i, row[:col])
		}
		if left == -1 {
			left = col
		} else if col != left {
			t.Errorf("logo line %d starts at col %d, want %d", i, col, left)
		}
	}
}

func firstLineWith(s, sub string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if strings.Contains(line, sub) {
			return line
		}
	}
	return ""
}
