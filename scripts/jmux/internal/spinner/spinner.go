// Package spinner shows a centered logo splash with a progress line while a slow
// operation runs — e.g. the gap between accepting a picker choice and the session
// opening. On a tty it paints on the alternate screen buffer, so the terminal is
// left untouched once the work finishes (and tmux takes over).
package spinner

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

var frames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// logo is the jmux wordmark (ANSI Shadow), drawn centered above the progress
// line. Lines may differ in length; the block is centered as a whole, so they
// keep their alignment.
var logo = []string{
	`     ██╗ ███╗   ███╗ ██╗   ██╗ ██╗  ██╗`,
	`     ██║ ████╗ ████║ ██║   ██║ ╚██╗██╔╝`,
	`     ██║ ██╔████╔██║ ██║   ██║  ╚███╔╝ `,
	`██   ██║ ██║╚██╔╝██║ ██║   ██║  ██╔██╗ `,
	`╚█████╔╝ ██║ ╚═╝ ██║ ╚██████╔╝ ██╔╝ ██╗`,
	` ╚════╝  ╚═╝     ╚═╝  ╚═════╝  ╚═╝  ╚═╝`,
}

const (
	altEnter = "\x1b[?1049h\x1b[?25l" // alternate screen + hide cursor
	altLeave = "\x1b[?25h\x1b[?1049l" // show cursor + restore screen
	// fg is the terminal's default foreground — i.e. the theme's primary color —
	// so the logo tracks whatever theme is active rather than a fixed palette
	// slot. dim mutes the action line beneath it for contrast.
	fg    = "\x1b[39m"
	dim   = "\x1b[2m"
	reset = "\x1b[0m"
)

// Run animates the splash on stderr while fn runs, starting with initial and
// switching to whatever fn sends on the phase channel, then restores the screen
// and returns fn's error. Without a tty it just drains phase and waits for fn.
func Run(initial string, fn func(phase chan<- string) error) error {
	phase := make(chan string, 1) // buffered so a phase send never blocks fn
	done := make(chan error, 1)
	go func() { done <- fn(phase) }()

	// tick stays nil off a tty, so the render cases never paint but the loop
	// still drains phase and waits for done — no special non-tty path needed.
	tty := isTerminal(os.Stderr)
	var tick <-chan time.Time
	if tty {
		fmt.Fprint(os.Stderr, altEnter)
		defer fmt.Fprint(os.Stderr, altLeave)
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		tick = t.C
	}

	msg := initial
	if tty {
		render(0, msg) // paint instantly rather than waiting for the first tick
	}
	for i := 0; ; i++ {
		select {
		case err := <-done:
			return err
		case msg = <-phase:
			if tty {
				render(i, msg)
			}
		case <-tick:
			render(i, msg)
		}
	}
}

// render repaints the alternate screen: the cyan logo block centered, then the
// spinner frame and current action centered just below it.
func render(i int, msg string) {
	width, height, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || width <= 0 || height <= 0 {
		width, height = 80, 24
	}

	block := 0
	for _, l := range logo {
		if w := utf8.RuneCountInString(l); w > block {
			block = w
		}
	}
	left := pad(width, block)

	// vertically center the logo + a blank gap + the action line.
	top := pad(height, len(logo)+2)

	// Overwrite in place rather than clearing the screen each tick (a full \x1b[2J
	// blanks for an instant, which flickers): home, rewrite each row erasing to its
	// end (\x1b[K), then clear below (\x1b[J) so a taller previous frame leaves none.
	var b strings.Builder
	b.WriteString("\x1b[H")
	row := func(s string) {
		b.WriteString(s)
		b.WriteString("\x1b[K\n")
	}
	for range top {
		row("")
	}
	for _, l := range logo {
		row(strings.Repeat(" ", left) + fg + l + reset)
	}
	row("") // gap between logo and action
	action := fmt.Sprintf("%c %s", frames[i%len(frames)], msg)
	b.WriteString(strings.Repeat(" ", pad(width, utf8.RuneCountInString(action))))
	b.WriteString(dim + action + reset)
	b.WriteString("\x1b[J")
	fmt.Fprint(os.Stderr, b.String())
}

// pad returns the left offset to center content of width inner within total,
// clamped at 0 so a too-narrow terminal just left-aligns.
func pad(total, inner int) int {
	if p := (total - inner) / 2; p > 0 {
		return p
	}
	return 0
}

// isTerminal reports whether f is a character device (a tty).
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
