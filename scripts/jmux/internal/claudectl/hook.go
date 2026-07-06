package claudectl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"jmux/internal/fzfutil"
	"jmux/internal/gitctl"
	"jmux/internal/tag"
	"jmux/internal/tmuxctl"
)

type hookInput struct {
	HookEventName    string `json:"hook_event_name"`
	NotificationType string `json:"notification_type"`
	Message          string `json:"message"`
	CWD              string `json:"cwd"`
}

// The hook_event_name values jmux is wired to in settings.json.
const (
	eventUserPromptSubmit = "UserPromptSubmit"
	eventNotification     = "Notification"
	eventStop             = "Stop"
	eventSessionEnd       = "SessionEnd"
)

// The notification_type values that mean claude is blocked on the user.
const (
	notifyPermissionPrompt  = "permission_prompt"
	notifyIdlePrompt        = "idle_prompt"
	notifyAgentNeedsInput   = "agent_needs_input"
	notifyElicitationDialog = "elicitation_dialog"
)

// RunHook handles `jmux claude hook`, the single entry point for the hook
// events wired in settings.json (UserPromptSubmit, Notification, Stop,
// SessionEnd). Every event updates the workspace's claude badge; a
// Notification also pushes an interruption.
func RunHook() error {
	var in hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		return fmt.Errorf("decoding hook input: %w", err)
	}
	status(in)
	if in.HookEventName == eventNotification {
		return push(in)
	}
	return nil
}

// status maps a hook event onto the workspace's "claude" badge, so the
// overview shows live session state.
func status(in hookInput) {
	if in.CWD == "" {
		return
	}
	// Badges key on workspace paths, but claude's cwd may be a subdirectory —
	// map it to the worktree root the overview actually lists.
	path := gitctl.Toplevel(in.CWD)
	if path == "" {
		path = in.CWD
	}
	switch in.HookEventName {
	case eventUserPromptSubmit:
		tag.Set(path, "claude", tag.Badge{Text: "claude ●", Color: tag.Green})
	case eventNotification:
		switch in.NotificationType {
		case notifyPermissionPrompt, notifyIdlePrompt, notifyAgentNeedsInput, notifyElicitationDialog:
			tag.Set(path, "claude", tag.Badge{Text: "claude needs input", Color: tag.Yellow})
		}
	case eventStop:
		tag.Set(path, "claude", tag.Badge{Text: "claude idle", Color: tag.Gray})
	case eventSessionEnd:
		tag.Unset(path, "claude")
	}
}

// push interrupts the user about a Notification: ping every attached client's
// status line, and post a macOS alert whose click jumps back to this pane.
func push(in hookInput) error {
	msg := in.Message
	if msg == "" {
		msg = "Needs your attention"
	}
	title := "Claude Code"
	switch in.NotificationType {
	case notifyPermissionPrompt:
		title = "Claude Code — Permission Required"
	case notifyIdlePrompt:
		title = "Claude Code — Waiting for Input"
	}

	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		pane = "%0"
	}
	target := tmuxctl.PaneTarget(pane)

	// The alert only helps when the terminal isn't frontmost; the client ping
	// covers being inside tmux, wherever the client is looking.
	for _, c := range tmuxctl.ListClients() {
		tmuxctl.DisplayToClient(c, fmt.Sprintf("claude (%s): %s", target, msg))
	}

	tn, err := exec.LookPath("terminal-notifier")
	if err != nil {
		return exec.Command("osascript", "-e",
			fmt.Sprintf("display notification %q with title %q", msg, title)).Run()
	}
	return exec.Command(tn,
		"-title", title,
		"-message", msg,
		"-execute", fmt.Sprintf("%s claude focus %s %s", fzfutil.Self(), target, pane),
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
