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

// ListSessions returns the names of all tmux sessions. Empty when the server
// isn't running or on error.
func ListSessions() []string {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// SetSessionOption sets a session-scoped option (e.g. a user option like
// "@jmux_dir") on the named session.
func SetSessionOption(name, key, value string) {
	exec.Command("tmux", "set-option", "-t", name, key, value).Run()
}

// SessionOption returns the value of a session option, or "" if unset.
func SessionOption(name, key string) string {
	out, err := exec.Command("tmux", "show-options", "-v", "-t", name, key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SessionPath returns the current path of the session's active pane, used as a
// fallback for sessions created before @jmux_dir stamping existed.
func SessionPath(name string) string {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", name, "#{pane_current_path}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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

// PanePIDs returns the PIDs of the processes running in session's panes (across
// all its windows).
func PanePIDs(session string) []int {
	out, err := exec.Command("tmux", "list-panes", "-s", "-t", session, "-F", "#{pane_pid}").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for l := range strings.FieldsSeq(string(out)) {
		if pid, err := strconv.Atoi(l); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
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

func SelectPane(target string) error {
	return exec.Command("tmux", "select-pane", "-t", target).Run()
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

// PaneVisible reports whether pane is currently on screen: it is the active
// pane of the active window of a session with at least one attached client.
// False for an empty pane id or when tmux can't resolve it.
func PaneVisible(pane string) bool {
	if pane == "" {
		return false
	}
	out, err := exec.Command("tmux", "display-message", "-p", "-t", pane,
		"#{pane_active}#{window_active}#{?session_attached,1,0}").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "111"
}

// PaneWindows maps every pane id to the index of its containing window, for
// resolving stored pane ids to display labels in one tmux call.
func PaneWindows() map[string]string {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_id} #{window_index}").Output()
	if err != nil {
		return nil
	}
	m := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if id, win, ok := strings.Cut(line, " "); ok {
			m[id] = win
		}
	}
	return m
}

// PaneTarget returns "session:window-index" of the window containing pane, or
// "" on error. Deliberately pane-targeted: an untargeted display-message would
// answer for the client's active window instead.
func PaneTarget(pane string) string {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", pane, "#S:#I").Output()
	if err != nil {
		return ""
	}
	target := strings.TrimSpace(string(out))
	// A pane tmux can't resolve still expands the format, with empty fields —
	// a bare ":" is no target.
	if strings.Trim(target, ":.") == "" {
		return ""
	}
	return target
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
