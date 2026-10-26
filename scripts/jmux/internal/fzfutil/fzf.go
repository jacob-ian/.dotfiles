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
	// Delimiter + WithNth hide trailing fields: only WithNth columns show, but
	// the full row is still returned on selection.
	Delimiter string
	WithNth   string
	// ANSI enables parsing of ANSI colour codes in the displayed rows.
	ANSI  bool
	Query string
}

// Row builds `display<TAB>path`: with Delimiter "\t" and WithNth "1" fzf shows
// only display, while bindings still get the path as {2}.
func Row(display, path string) string {
	return display + "\t" + path
}

func RowPath(line string) string {
	if i := strings.LastIndexByte(line, '\t'); i >= 0 {
		return line[i+1:]
	}
	return line
}

// Self returns the path of the running jmux binary, for commands that
// re-invoke it (fzf bindings, notification callbacks); falls back to "jmux"
// (resolved via PATH) on error.
func Self() string {
	self, err := os.Executable()
	if err != nil {
		return "jmux"
	}
	return self
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
	if opts.Delimiter != "" {
		args = append(args, "--delimiter="+opts.Delimiter)
	}
	if opts.WithNth != "" {
		args = append(args, "--with-nth="+opts.WithNth)
	}
	if opts.ANSI {
		args = append(args, "--ansi")
	}
	if opts.Query != "" {
		args = append(args, "--query="+opts.Query)
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
