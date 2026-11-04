package tmuxctl

import (
	"os"
	"os/exec"
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

// ListClients returns the names of all attached tmux clients.
func ListClients() []string {
	out, err := exec.Command("tmux", "list-clients", "-F", "#{client_name}").Output()
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// ClientPIDs returns the pids of all attached tmux clients.
func ClientPIDs() []int {
	out, err := exec.Command("tmux", "list-clients", "-F", "#{client_pid}").Output()
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

// SetGlobalOption sets a server-wide option.
func SetGlobalOption(key, value string) {
	exec.Command("tmux", "set-option", "-g", key, value).Run()
}

// RefreshStatus redraws the named client's status line.
func RefreshStatus(client string) {
	exec.Command("tmux", "refresh-client", "-S", "-t", client).Run()
}

// SetSessionOption sets a session-scoped option.
func SetSessionOption(name, key, value string) {
	exec.Command("tmux", "set-option", "-t", name, key, value).Run()
}

// SessionDirs returns every session's originating directory in one call: the
// @jmux_dir stamp, else the active pane's path.
func SessionDirs() []string {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{@jmux_dir}\t#{pane_current_path}").Output()
	if err != nil {
		return nil
	}
	var dirs []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		stamp, pane, _ := strings.Cut(line, "\t")
		if stamp == "" {
			stamp = strings.TrimSpace(pane)
		}
		if stamp != "" {
			dirs = append(dirs, stamp)
		}
	}
	return dirs
}

// WindowNames returns the names of all windows in session. Empty on error.
func WindowNames(session string) []string {
	out, err := exec.Command("tmux", "list-windows", "-t="+session, "-F", "#{window_name}").Output()
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
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

// CapturePane returns target's visible buffer plus up to scrollback lines of history.
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

// PanePIDs returns the PIDs of the processes in session's panes.
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

// ShellQuote single-quotes s for use inside a tmux or fzf shell command.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

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

// SwitchClientTo points the named client at target; empty client lets tmux pick.
func SwitchClientTo(client, target string) error {
	args := []string{"switch-client"}
	if client != "" {
		args = append(args, "-c", client)
	}
	return exec.Command("tmux", append(args, "-t", target)...).Run()
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

// CurrentSession is the session containing this process, or "" outside tmux.
func CurrentSession() string {
	return current("#S")
}

// CurrentWindow is the window containing this process, or "" outside tmux.
func CurrentWindow() string {
	return current("#W")
}

// PaneVisible reports whether pane is on screen: the active pane of the
// active window of a session with an attached client.
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

// PaneWindows maps every pane id to its window index in one tmux call.
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

// PaneLabels maps every pane id to "session:window-index", which doubles as
// a switch-client target.
func PaneLabels() map[string]string {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_id} #{session_name}:#{window_index}").Output()
	if err != nil {
		return nil
	}
	m := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		// Cut on the first space: pane ids never contain one, session names may.
		if id, label, ok := strings.Cut(line, " "); ok {
			m[id] = label
		}
	}
	return m
}

// PaneTarget returns pane's "session:window-index". Pane-targeted on purpose:
// an untargeted display-message answers for the client's active window.
func PaneTarget(pane string) string {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", pane, "#S:#I").Output()
	if err != nil {
		return ""
	}
	target := strings.TrimSpace(string(out))
	// An unresolvable pane still expands the format with empty fields.
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
