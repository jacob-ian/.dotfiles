package tmuxctl

import (
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

func DisplayMessage(msg string) {
	exec.Command("tmux", "display-message", msg).Run()
}

// DisplayMessageFor shows msg for ms milliseconds (overrides display-time).
func DisplayMessageFor(msg string, ms int) {
	exec.Command("tmux", "display-message", "-d", strconv.Itoa(ms), msg).Run()
}

func HasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t="+name).Run() == nil
}

// WindowNames returns the names of all windows in session. Empty on error.
func WindowNames(session string) []string {
	out, err := exec.Command("tmux", "list-windows", "-t="+session, "-F", "#{window_name}").Output()
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

func HasWindow(session, window string) bool {
	return slices.Contains(WindowNames(session), window)
}

// CountWindows returns the number of windows in session named exactly window.
func CountWindows(session, window string) int {
	n := 0
	for _, w := range WindowNames(session) {
		if w == window {
			n++
		}
	}
	return n
}

// CapturePane returns the visible buffer of target (e.g. "session:window"),
// including up to `scrollback` extra lines of history. ANSI escapes are
// stripped.
func CapturePane(target string, scrollback int) string {
	args := []string{"capture-pane", "-p", "-t", target}
	if scrollback > 0 {
		args = append(args, "-S", "-"+strconv.Itoa(scrollback))
	}
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

func KillSession(name string) {
	exec.Command("tmux", "kill-session", "-t="+name).Run()
}

func NewSession(name, cwd, windowName, cmd string) error {
	args := []string{"new-session", "-ds", name}
	if windowName != "" {
		args = append(args, "-n", windowName)
	}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if cmd != "" {
		args = append(args, cmd)
	}
	return exec.Command("tmux", args...).Run()
}

// NewWindow creates a new window in session `target`. If `detached` is true the
// window is created without selecting it.
func NewWindow(target, name, cwd, cmd string, detached bool) error {
	args := []string{"new-window"}
	if detached {
		args = append(args, "-d")
	}
	args = append(args, "-t", target+":")
	if name != "" {
		args = append(args, "-n", name)
	}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if cmd != "" {
		args = append(args, cmd)
	}
	return exec.Command("tmux", args...).Run()
}

func SelectWindow(target string) error {
	return exec.Command("tmux", "select-window", "-t", target).Run()
}

func SwitchClient(name string) error {
	return exec.Command("tmux", "switch-client", "-t", name).Run()
}

func Attach(name string) error {
	cmd := exec.Command("tmux", "attach", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func InsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// CurrentSession returns the name of the tmux session containing the running
// process, or "" when not inside tmux.
func CurrentSession() string {
	return current("#S")
}

// CurrentWindow returns the name of the tmux window containing the running
// process, or "" when not inside tmux.
func CurrentWindow() string {
	return current("#W")
}

func current(fmt string) string {
	if !InsideTmux() {
		return ""
	}
	out, err := exec.Command("tmux", "display-message", "-p", fmt).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
