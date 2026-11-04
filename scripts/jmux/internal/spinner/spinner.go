// Package spinner shows a logo splash with a progress line while a slow
// operation runs, on the alternate screen so the terminal is left untouched.
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
	// fg is the terminal's default foreground, so the logo tracks the active theme.
	fg    = "\x1b[39m"
	dim   = "\x1b[2m"
	reset = "\x1b[0m"
)

// Run animates the splash on stderr while fn runs, showing initial and then
// whatever fn sends on phase. Without a tty it just waits for fn.
func Run(initial string, fn func(phase chan<- string) error) error {
	phase := make(chan string, 1) // buffered so a phase send never blocks fn
	done := make(chan error, 1)
	go func() { done <- fn(phase) }()

	// tick stays nil off a tty, so the loop never paints but still drains phase.
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
		render(0, msg)
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

	top := pad(height, len(logo)+2)

	// Rewrite in place rather than clearing: a full clear blanks for an instant and flickers.
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
	row("")
	action := fmt.Sprintf("%c %s", frames[i%len(frames)], msg)
	b.WriteString(strings.Repeat(" ", pad(width, utf8.RuneCountInString(action))))
	b.WriteString(dim + action + reset)
	b.WriteString("\x1b[J")
	fmt.Fprint(os.Stderr, b.String())
}

func pad(total, inner int) int {
	if p := (total - inner) / 2; p > 0 {
		return p
	}
	return 0
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
