package claudectl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"jmux/internal/fzfutil"
	"jmux/internal/notify"
	"jmux/internal/repo"
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

const (
	eventUserPromptSubmit = "UserPromptSubmit"
	eventNotification     = "Notification"
	eventStop             = "Stop"
	eventSessionEnd       = "SessionEnd"
)

// Events that mean the user answered a prompt. Approvals have no hook of
// their own, so the approved tool completing stands in for one.
const (
	eventPostToolUse        = "PostToolUse"
	eventPostToolUseFailure = "PostToolUseFailure"
	eventPermissionDenied   = "PermissionDenied"
	eventElicitationResult  = "ElicitationResult"
)

const (
	notifyPermissionPrompt  = "permission_prompt"
	notifyIdlePrompt        = "idle_prompt"
	notifyAgentNeedsInput   = "agent_needs_input"
	notifyElicitationDialog = "elicitation_dialog"
)

type sessionStatus string

const (
	statusWorking    sessionStatus = "working"
	statusNeedsInput sessionStatus = "needs_input"
	statusIdle       sessionStatus = "idle"
)

const tagKind = "claude"

const kindNeedsInput = tagKind + ".needs_input"

type tagData struct {
	Status    sessionStatus `json:"status"`
	UpdatedAt time.Time     `json:"updated_at,omitzero"`
}

var registerOnce sync.Once

// Register wires this package's workspace-tag renderer and statusbox notice source.
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

// Status is the state of the newest live claude session in path's workspace:
// "working", "needs_input", "idle", or "" when none is running.
func Status(path string) string {
	tags, ok := tag.All()[repo.Resolve(path)]
	if !ok {
		return ""
	}
	return string(newestStatus(tags, tmuxctl.PaneLabels()))
}

func newestStatus(tags []tag.Tag, panes map[string]string) sessionStatus {
	var newest tagData
	for _, t := range tags {
		if t.Kind != tagKind || t.Pane == "" || panes[t.Pane] == "" {
			continue
		}
		var d tagData
		if json.Unmarshal(t.Data, &d) == nil && d.UpdatedAt.After(newest.UpdatedAt) {
			newest = d
		}
	}
	return newest.Status
}

// noticesFromTags maps needs_input tags with a live pane to notices. The newest
// session per pane wins, so a successor session supersedes a stale claim.
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

// RunHook handles `jmux claude hook`, the entry point for every claude hook
// event wired in settings.json.
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

// status updates the session's claude badge for a hook event and reports
// whether it changed.
func status(in hookInput) bool {
	if in.CWD == "" {
		return false
	}
	path := repo.WorkRoot(in.CWD)
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
		// PostToolUse fires on every tool call, so the common case must stay a
		// single cache read.
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

// push interrupts the user about a Notification unless its pane is already on screen.
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

// RunFocus handles `jmux claude focus <pane>`, the alert's click callback.
// terminal-notifier runs it with a minimal PATH, hence the homebrew append.
func RunFocus(args []string) error {
	if len(args) < 1 || args[0] == "" {
		return errors.New("usage: jmux claude focus <pane>")
	}
	os.Setenv("PATH", os.Getenv("PATH")+":/opt/homebrew/bin")
	notify.ActivateTerminal()
	return focusPane(args[0], "")
}
