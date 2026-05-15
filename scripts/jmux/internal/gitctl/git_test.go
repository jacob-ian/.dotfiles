package gitctl

import (
	"errors"
	"testing"
)

func TestCleanErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"strips fatal", errors.New("fatal: invalid reference: foo"), "invalid reference: foo"},
		{"trims whitespace", errors.New("  fatal: x  \n"), "x"},
		{"first line only", errors.New("fatal: first\nsecond\nthird"), "first"},
		{"no fatal prefix", errors.New("error: something"), "error: something"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanErr(tt.err); got != tt.want {
				t.Errorf("CleanErr(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
