package fzfutil

import "testing"

func TestRowPath(t *testing.T) {
	cases := map[string]string{
		Row("/repo/feature  ·  PR #42", "/repo/feature"):    "/repo/feature",
		Row("✎ /scratch/2026-10-26", "/scratch/2026-10-26"): "/scratch/2026-10-26",
		"/no/tab/field": "/no/tab/field",
	}
	for in, want := range cases {
		if got := RowPath(in); got != want {
			t.Errorf("RowPath(%q) = %q, want %q", in, got, want)
		}
	}
}
