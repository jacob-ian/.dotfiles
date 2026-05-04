package tmuxctl

import (
	"os"
	"os/exec"
	"strings"
)

func DisplayMessage(msg string) {
	exec.Command("tmux", "display-message", msg).Run()
}

func HasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t="+name).Run() == nil
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
	if !InsideTmux() {
		return ""
	}
	out, err := exec.Command("tmux", "display-message", "-p", "#S").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
