// Package spinner shows an inline progress animation while a slow operation
// runs — e.g. the gap between accepting a picker choice and the session opening.
package spinner

import (
	"fmt"
	"os"
	"time"
)

var frames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// Run animates a spinner on stderr while fn runs, starting with initial and
// switching to whatever fn sends on the phase channel, then clears the line and
// returns fn's error. Without a tty it just drains phase and waits for fn; work
// shorter than one frame shows nothing.
func Run(initial string, fn func(phase chan<- string) error) error {
	phase := make(chan string, 1) // buffered so a phase send never blocks fn
	done := make(chan error, 1)
	go func() { done <- fn(phase) }()

	// tick stays nil off a tty, so the render case never fires but the loop
	// still drains phase and waits for done — no special non-tty path needed.
	var tick <-chan time.Time
	if isTerminal(os.Stderr) {
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		tick = t.C
	}
	msg := initial
	for i := 0; ; i++ {
		select {
		case err := <-done:
			if tick != nil {
				fmt.Fprint(os.Stderr, "\r\x1b[K") // erase the spinner line
			}
			return err
		case msg = <-phase:
		case <-tick:
			fmt.Fprintf(os.Stderr, "\r\x1b[K%c %s", frames[i%len(frames)], msg)
		}
	}
}

// isTerminal reports whether f is a character device (a tty).
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
