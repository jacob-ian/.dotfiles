package claudectl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"jmux/internal/fzfutil"
	"jmux/internal/tmuxctl"
)

type hookInput struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// RunNotify handles `jmux claude notify`, the Claude Code Notification hook:
// post a macOS alert whose click jumps back to this pane.
func RunNotify() error {
	var in hookInput
	_ = json.NewDecoder(os.Stdin).Decode(&in) // malformed input still notifies
	if in.Message == "" {
		in.Message = "Needs your attention"
	}
	title := "Claude Code"
	switch in.Type {
	case "permission_prompt":
		title = "Claude Code — Permission Required"
	case "idle_prompt":
		title = "Claude Code — Waiting for Input"
	}

	tn, err := exec.LookPath("terminal-notifier")
	if err != nil {
		return exec.Command("osascript", "-e",
			fmt.Sprintf("display notification %q with title %q", in.Message, title)).Run()
	}

	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		pane = "%0"
	}
	return exec.Command(tn,
		"-title", title,
		"-message", in.Message,
		"-execute", fmt.Sprintf("%s claude focus %s %s", fzfutil.Self(), tmuxctl.PaneTarget(pane), pane),
	).Run()
}

// RunFocus handles `jmux claude focus <session:window> <pane>`, the alert's
// click callback. terminal-notifier runs it with a minimal PATH, hence the
// homebrew append.
func RunFocus(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: jmux claude focus <session:window> <pane>")
	}
	os.Setenv("PATH", os.Getenv("PATH")+":/opt/homebrew/bin")
	_ = exec.Command("open", "-a", "kitty").Run()
	if err := tmuxctl.SwitchClient(args[0]); err != nil {
		return fmt.Errorf("switching to %s: %w", args[0], err)
	}
	return tmuxctl.SelectPane(args[1])
}
