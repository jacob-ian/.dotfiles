package fzfutil

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

type Options struct {
	Prompt        string
	Header        string
	Bindings      []string
	Preview       string
	PreviewWindow string
}

// Pick runs fzf on items and returns the selected line. Returns "" with a
// non-nil error when the user cancels or fzf otherwise exits non-zero.
func Pick(items []string, opts Options) (string, error) {
	out, err := runFzf(items, buildArgs(opts, false))
	if err != nil {
		return "", err
	}
	return out, nil
}

// PickOrQuery runs fzf with --print-query, returning either the selection or
// the user's typed query when nothing matched. Useful for "pick from this
// list, or type a new value" flows.
//
// fzf exits non-zero when no item matches; that's expected here, so the error
// is swallowed as long as a usable string came back.
func PickOrQuery(items []string, opts Options) (string, error) {
	out, err := runFzf(items, buildArgs(opts, true))
	// --print-query prints the query on line 1 and any selection on later
	// lines. The last non-empty line is the selection if one exists, else the
	// query itself.
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s, nil
		}
	}
	return "", err
}

func runFzf(items, args []string) (string, error) {
	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(items, "\n"))
	cmd.Stderr = os.Stderr
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), err
}

func buildArgs(opts Options, printQuery bool) []string {
	var args []string
	if opts.Prompt != "" {
		args = append(args, "--prompt="+opts.Prompt)
	}
	if opts.Header != "" {
		args = append(args, "--header="+opts.Header)
	}
	for _, b := range opts.Bindings {
		args = append(args, "--bind="+b)
	}
	if printQuery {
		args = append(args, "--print-query")
	}
	if opts.Preview != "" {
		args = append(args, "--preview="+opts.Preview)
	}
	if opts.PreviewWindow != "" {
		args = append(args, "--preview-window="+opts.PreviewWindow)
	}
	return args
}
