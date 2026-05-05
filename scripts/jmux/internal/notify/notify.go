// Package notify routes user-facing status messages to tmux when running
// inside a session, and to stderr otherwise.
package notify

import (
	"fmt"
	"os"

	"jmux/internal/tmuxctl"
)

// Info reports an informational message.
func Info(msg string) {
	if tmuxctl.InsideTmux() {
		tmuxctl.DisplayMessage(msg)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}

// errorDisplayMs holds errors on the tmux status line longer than the default
// display-time so they don't flash by.
const errorDisplayMs = 4000

// Error reports an error, rendered red+bold inside tmux.
func Error(msg string) {
	if tmuxctl.InsideTmux() {
		tmuxctl.DisplayMessageFor("#[fg=red,bold]✖#[fg=black,bold] "+msg, errorDisplayMs)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}
