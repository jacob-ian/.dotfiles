package claudectl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

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
	SessionID        string `json:"session_id"`
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

// sessionStatus is a session's state as reported by its hooks — the claude
// tag's payload.
type sessionStatus string

const (
	statusWorking    sessionStatus = "working"
	statusNeedsInput sessionStatus = "needs_input"
	statusIdle       sessionStatus = "idle"
)

// tagKind doubles as the namespace prefix: session namespaces are
// "claude:<session_id>".
const tagKind = "claude"

type tagData struct {
	Status sessionStatus `json:"status"`
}

var registerTagOnce sync.Once

// RegisterTag wires this package's workspace-tag renderer; idempotent so main
// and tests can both call it.
func RegisterTag() {
	registerTagOnce.Do(func() {
		tag.Register(tagKind, func(data json.RawMessage) (string, tag.Color) {
			var d tagData
			if json.Unmarshal(data, &d) != nil {
				return "", ""
			}
			switch d.Status {
			case statusWorking:
				return "✻ working", tag.Green
			case statusNeedsInput:
				return "✻ needs input", tag.Yellow
			case statusIdle:
				return "✻ idle", tag.Gray
			}
			return "", ""
		})
	})
}

func claudeTag(status sessionStatus, pane string) tag.Tag {
	return tag.New(tagKind, pane, tagData{Status: status})
}

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

// status maps a hook event onto the workspace's claude badge for the session
// that fired it, so the overview shows live state per agent. The namespace
// carries the session id so concurrent agents in one worktree don't clobber
// each other's badge.
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
	ns := tagKind
	if in.SessionID != "" {
		ns += ":" + in.SessionID
	}
	pane := os.Getenv("TMUX_PANE")
	switch in.HookEventName {
	case eventUserPromptSubmit:
		tag.Set(path, ns, claudeTag(statusWorking, pane))
	case eventNotification:
		switch in.NotificationType {
		case notifyPermissionPrompt, notifyIdlePrompt, notifyAgentNeedsInput, notifyElicitationDialog:
			tag.Set(path, ns, claudeTag(statusNeedsInput, pane))
		}
	case eventStop:
		tag.Set(path, ns, claudeTag(statusIdle, pane))
	case eventSessionEnd:
		tag.Unset(path, ns)
	}
}

// push interrupts the user about a Notification: ping every attached client's
// status line, and post a macOS alert whose click jumps back to this pane.
// Skipped entirely when the pane is already on screen — the user is looking
// at the prompt the notification would point them to.
func push(in hookInput) error {
	if tmuxctl.PaneVisible(os.Getenv("TMUX_PANE")) {
		return nil
	}
	title := "jmux"
	msg := in.Message
	if msg == "" {
		switch in.NotificationType {
		case notifyPermissionPrompt:
			msg = "Claude needs permission."
		case notifyIdlePrompt:
			msg = "Claude is waiting for input."
		case notifyAgentNeedsInput:
			msg = "An agent needs input."
		case notifyElicitationDialog:
			msg = "Claude is asking a question."
		default:
			msg = "Needs your attention."
		}
	}
	// Claude's own messages arrive without terminal punctuation.
	if !strings.HasSuffix(msg, ".") && !strings.HasSuffix(msg, "?") && !strings.HasSuffix(msg, "!") {
		msg += "."
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
		"-message", msg+"\nClick to jump to the pane.",
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
