// Package notify routes messages: Info/Error are passive (tmux status line
// or stderr); Interrupt reaches the user via a macOS alert.
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

// errorDisplayMs holds errors longer than display-time so they don't flash by.
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

// Interrupt posts a macOS alert whose click runs onClick, unless the terminal
// is frontmost (the multibox is already in view). cta is appended only when
// the alert is clickable.
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

// plistSafe defeats terminal-notifier's NSUserDefaults parsing: a value
// starting with (, {, [ or " is read as a plist and lost. Ordinary whitespace
// is skipped, so the guard has to be a zero-width space.
func plistSafe(s string) string {
	if s != "" && strings.ContainsAny(s[:1], `([{"`) {
		return "\u200b" + s
	}
	return s
}

// alert uses terminal-notifier when available (the only clickable path), else osascript.
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
