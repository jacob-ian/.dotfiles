package claudectl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"jmux/internal/fzfutil"
	"jmux/internal/gitctl"
	"jmux/internal/notify"
	"jmux/internal/session"
	"jmux/internal/statusbox"
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

// The hook events that mean the user answered whatever claude was blocked on.
// Approvals have no decision-time hook, so the approved tool completing (or
// failing) stands in for one; denials and elicitation answers do fire at
// decision time.
const (
	eventPostToolUse        = "PostToolUse"
	eventPostToolUseFailure = "PostToolUseFailure"
	eventPermissionDenied   = "PermissionDenied"
	eventElicitationResult  = "ElicitationResult"
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

// kindNeedsInput is the statusbox notice kind — the event, not the domain,
// so dismissing it never suppresses other claude notice types.
const kindNeedsInput = tagKind + ".needs_input"

type tagData struct {
	Status    sessionStatus `json:"status"`
	UpdatedAt time.Time     `json:"updated_at,omitzero"`
}

var registerOnce sync.Once

// Register wires this package's workspace-tag renderer and statusbox notice
// source; idempotent so main and tests can both call it.
func Register() {
	registerOnce.Do(func() {
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
		statusbox.Source(kindNeedsInput, func() []statusbox.Notice {
			return noticesFromTags(tag.All(), tmuxctl.PaneLabels())
		})
		statusbox.Handler(kindNeedsInput, func(n statusbox.Notice, client string) error {
			return focusPane(n.ID, client)
		})
	})
}

// noticesFromTags maps needs_input tags to notices labelled by workspace,
// dropping tags with no pane to jump to (unset, or absent from the live
// panes). Deduping per pane keeps the most recently updated session, so a
// newer quiet session supersedes a stale claim from a predecessor in the
// same pane.
func noticesFromTags(tags map[string][]tag.Tag, panes map[string]string) []statusbox.Notice {
	type claim struct {
		d    tagData
		path string
	}
	newest := map[string]claim{}
	for path, ts := range tags {
		for _, t := range ts {
			if t.Kind != tagKind || t.Pane == "" || panes[t.Pane] == "" {
				continue
			}
			var d tagData
			if json.Unmarshal(t.Data, &d) != nil {
				continue
			}
			if prev, ok := newest[t.Pane]; !ok || d.UpdatedAt.After(prev.d.UpdatedAt) {
				newest[t.Pane] = claim{d: d, path: path}
			}
		}
	}
	var out []statusbox.Notice
	for pane, c := range newest {
		if c.d.Status != statusNeedsInput {
			continue
		}
		out = append(out, statusbox.Notice{
			ID:     pane,
			Label:  session.DisplayName(c.path),
			Verb:   "needs input",
			Plural: "need input",
			Since:  c.d.UpdatedAt,
		})
	}
	return out
}

// focusPane jumps the client to the pane, resolving its window at jump time
// so a pane that moved still focuses correctly.
func focusPane(pane, client string) error {
	target := tmuxctl.PaneTarget(pane)
	if target == "" {
		return errors.New("pane is gone")
	}
	if err := tmuxctl.SwitchClientTo(client, target); err != nil {
		return fmt.Errorf("switching to %s: %w", target, err)
	}
	return tmuxctl.SelectPane(pane)
}

func claudeTag(status sessionStatus, pane string) tag.Tag {
	return tag.New(tagKind, pane, tagData{Status: status, UpdatedAt: time.Now()})
}

// RunHook handles `jmux claude hook`, the single entry point for the hook
// events wired in settings.json (UserPromptSubmit, Notification, Stop,
// SessionEnd, and the answered events). Events that change the workspace's
// claude badge republish the statusbox; a Notification also pushes an
// interruption.
func RunHook() error {
	var in hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		return fmt.Errorf("decoding hook input: %w", err)
	}
	if status(in) {
		statusbox.Publish()
	}
	if in.HookEventName == eventNotification {
		return push(in)
	}
	return nil
}

// status maps a hook event onto the workspace's claude badge for the session
// that fired it, so the overview shows live state per agent. The namespace
// carries the session id so concurrent agents in one worktree don't clobber
// each other's badge. Reports whether the badge changed, so no-op events
// skip the statusbox republish.
func status(in hookInput) bool {
	if in.CWD == "" {
		return false
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
		default:
			return false
		}
	case eventPostToolUse, eventPostToolUseFailure, eventPermissionDenied, eventElicitationResult:
		// PostToolUse fires on every tool call, so the common case — already
		// working, mid agentic run — must stay a single cache read.
		if currentStatus(path, ns) == statusWorking {
			return false
		}
		tag.Set(path, ns, claudeTag(statusWorking, pane))
	case eventStop:
		tag.Set(path, ns, claudeTag(statusIdle, pane))
	case eventSessionEnd:
		tag.Unset(path, ns)
	default:
		return false
	}
	return true
}

// currentStatus reads the session's current badge status, "" when unset or
// undecodable.
func currentStatus(path, ns string) sessionStatus {
	t, ok := tag.Get(path, ns)
	if !ok {
		return ""
	}
	var d tagData
	if json.Unmarshal(t.Data, &d) != nil {
		return ""
	}
	return d.Status
}

// push interrupts the user about a Notification, unless the pane is already
// on screen — the user is looking at the prompt the notification would point
// them to. (Interrupt itself stays quiet whenever the terminal is frontmost.)
func push(in hookInput) error {
	if tmuxctl.PaneVisible(os.Getenv("TMUX_PANE")) {
		return nil
	}
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

	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		pane = "%0"
	}
	return notify.Interrupt(tmuxctl.PaneTarget(pane), msg,
		"Click to jump to the pane.",
		fzfutil.Self()+" claude focus "+pane)
}

// RunFocus handles `jmux claude focus <pane>`, the macOS alert's click
// callback. terminal-notifier runs it with a minimal PATH, hence the
// homebrew append.
func RunFocus(args []string) error {
	if len(args) < 1 || args[0] == "" {
		return errors.New("usage: jmux claude focus <pane>")
	}
	os.Setenv("PATH", os.Getenv("PATH")+":/opt/homebrew/bin")
	notify.ActivateTerminal()
	return focusPane(args[0], "")
}
