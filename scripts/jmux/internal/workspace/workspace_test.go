package workspace

import "testing"

func TestRowPath(t *testing.T) {
	cases := map[string]string{
		"/repo/feature  ·  PR #42\t/repo/feature": "/repo/feature",
		"/repo/plain\t/repo/plain":                "/repo/plain",
		"/no/tab/field":                           "/no/tab/field",
	}
	for in, want := range cases {
		if got := rowPath(in); got != want {
			t.Errorf("rowPath(%q) = %q, want %q", in, got, want)
		}
	}
}
