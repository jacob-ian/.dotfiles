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
	PrintQuery    bool
	Preview       string
	PreviewWindow string
}

// Run pipes items into fzf and returns the trimmed selection.
// When PrintQuery is true, the output is the user's typed query if no item was
// chosen (last line of fzf's stdout).
func Run(items []string, opts Options) (string, error) {
	args := buildArgs(opts)
	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(items, "\n"))
	cmd.Stderr = os.Stderr
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if opts.PrintQuery {
		// fzf with --print-query prints the query on line 1 and any selected
		// items on subsequent lines. Last non-empty line is what we want
		// (selection if any, else the query itself).
		// fzf exits non-zero when there's no match — that's expected here, so
		// we swallow the error as long as we have a usable string.
		lines := strings.Split(out, "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if s := strings.TrimSpace(lines[i]); s != "" {
				return s, nil
			}
		}
		return "", err
	}
	if err != nil {
		return "", err
	}
	return out, nil
}

func buildArgs(opts Options) []string {
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
	if opts.PrintQuery {
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
