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
	// Delimiter + WithNth show only those columns; the full row is still returned.
	Delimiter string
	WithNth   string
	ANSI      bool
	Query     string
}

// Row builds display<TAB>path; with Delimiter "\t" and WithNth "1" bindings get the path as {2}.
func Row(display, path string) string {
	return display + "\t" + path
}

func RowPath(line string) string {
	if i := strings.LastIndexByte(line, '\t'); i >= 0 {
		return line[i+1:]
	}
	return line
}

// Self is the running binary's path, for commands that re-invoke it; "jmux" on error.
func Self() string {
	self, err := os.Executable()
	if err != nil {
		return "jmux"
	}
	return self
}

// Pick runs fzf on items; a cancel returns "" and an error.
func Pick(items []string, opts Options) (string, error) {
	out, err := runFzf(items, buildArgs(opts, false))
	if err != nil {
		return "", err
	}
	return out, nil
}

// PickOrQuery returns the selection, or the typed query when nothing matched.
// fzf exits non-zero on no match, so that error is swallowed when a string came back.
func PickOrQuery(items []string, opts Options) (string, error) {
	out, err := runFzf(items, buildArgs(opts, true))
	// --print-query puts the query on line 1 and the selection after; the last non-empty line wins.
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
