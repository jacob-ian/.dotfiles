// Package notify routes user-facing messages at two tiers: Info/Error are
// passive status messages shown in the current view (tmux status line inside
// a session, stderr otherwise); Interrupt reaches the user wherever they are
// looking via a macOS alert.
package notify

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"jmux/internal/tmuxctl"
)

func Info(msg string) {
	if tmuxctl.InsideTmux() {
		tmuxctl.DisplayMessage(msg)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}

func Infof(format string, args ...any) {
	Info(fmt.Sprintf(format, args...))
}

// errorDisplayMs holds errors on the tmux status line longer than the default
// display-time so they don't flash by.
const errorDisplayMs = 4000

// Error renders msg red+bold when displayed inside tmux.
func Error(msg string) {
	if tmuxctl.InsideTmux() {
		tmuxctl.DisplayMessageFor("#[fg=red,bold]✖#[fg=black,bold] "+msg, errorDisplayMs)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}

func Errorf(format string, args ...any) {
	Error(fmt.Sprintf(format, args...))
}

// Interrupt reaches the user wherever they are looking via a macOS alert
// whose click runs onClick — unless the terminal itself is frontmost, where
// passive signals (the status-line multibox) are already in view and an
// alert would duplicate them. The title is "jmux - <source>" (or just "jmux"
// when source is empty). cta is appended to the alert body on its own line
// only when the delivery mechanism supports clicking.
func Interrupt(source, body, cta, onClick string) error {
	if terminalFocused() {
		return nil
	}
	title := "jmux"
	if source != "" {
		title += " - " + source
	}
	return alert(title, punctuate(body), cta, onClick)
}

func punctuate(s string) string {
	if s == "" || strings.ContainsAny(s[len(s)-1:], ".?!") {
		return s
	}
	return s + "."
}

// plistSafe defeats terminal-notifier's NSUserDefaults argument parsing: a
// value starting with (, {, [, or " is read as a property-list collection
// and the text is lost. The parser skips ordinary whitespace, so the
// invisible guard has to be a zero-width space.
func plistSafe(s string) string {
	if s != "" && strings.ContainsAny(s[:1], `([{"`) {
		return "\u200b" + s
	}
	return s
}

// alert posts the macOS notification: terminal-notifier when available (the
// only clickable path, so the only one that carries cta and onClick), plain
// osascript otherwise.
func alert(title, body, cta, onClick string) error {
	tn, err := exec.LookPath("terminal-notifier")
	if err != nil {
		return exec.Command("osascript", "-e",
			fmt.Sprintf("display notification %q with title %q", body, title)).Run()
	}
	if cta != "" {
		body += "\n" + cta
	}
	args := []string{"-title", plistSafe(title), "-message", plistSafe(body)}
	if onClick != "" {
		args = append(args, "-execute", onClick)
	}
	return exec.Command(tn, args...).Run()
}
