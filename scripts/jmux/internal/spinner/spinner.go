// Package spinner shows an inline progress animation while a slow operation
// runs — e.g. the gap between accepting a picker choice and the session opening.
package spinner

import (
	"fmt"
	"os"
	"time"
)

var frames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// Run animates msg on stderr while fn runs, clears the line, and returns fn's
// error. Without a tty it just runs fn; work shorter than one frame shows nothing.
func Run(msg string, fn func() error) error {
	if !isTerminal(os.Stderr) {
		return fn()
	}
	done := make(chan error, 1)
	go func() { done <- fn() }()

	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()
	for i := 0; ; i++ {
		select {
		case err := <-done:
			fmt.Fprint(os.Stderr, "\r\x1b[K") // erase the spinner line
			return err
		case <-tick.C:
			fmt.Fprintf(os.Stderr, "\r%c %s", frames[i%len(frames)], msg)
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
