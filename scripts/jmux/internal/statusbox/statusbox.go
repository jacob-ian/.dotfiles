// Package statusbox renders the tmux status-line multibox: the newest
// undismissed notice as a clickable notification, a summary count once
// everything is dismissed, nothing when nothing claims attention. A notice
// kind is an event type wiring a Source (derive its current notices) and a
// Handler (act on a clicked one). Render and click both derive from scratch,
// so the persisted state carries only data, never actions, and dismissals
// self-invalidate by comparison against truth. The box is published to a
// global tmux user option that status-right references, so one render
// reaches every session.
package statusbox

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"jmux/internal/cachefile"
	"jmux/internal/tmuxctl"
)

// boxOption is the tmux user option the status line references via
// #{E:@jmux_statusbox}; rangeContent/rangeX are the user-range names the
// MouseDown1Status binding dispatches on. tmux caps range arguments at 15
// bytes; only the displayed notification carries ranges, so fixed names
// suffice and a click resolves through the persisted display entry.
const (
	boxOption    = "@jmux_statusbox"
	rangeContent = "jmux-content"
	rangeX       = "jmux-x"
)

const stateFile = "statusbox.json"

// Notice is one item claiming the user's attention — pure data (it persists
// to the state file whole); behavior lives in the kind's Handler.
type Notice struct {
	Kind   string    `json:"kind"`             // event type, e.g. "claude.needs_input"; stamped by collect, never by sources
	ID     string    `json:"id"`               // identity within the kind (claude: pane id); Kind+ID is the notice's key
	Label  string    `json:"label,omitempty"`  // display text; escaped and truncated at render time
	Verb   string    `json:"verb,omitempty"`   // singular claim, e.g. "needs input"
	Plural string    `json:"plural,omitempty"` // summary form for counts above one; empty reuses Verb
	Since  time.Time `json:"since,omitzero"`   // when the claim last renewed; zero never renews, so dismissals stick
}

func (n Notice) key() string {
	return n.Kind + " " + n.ID
}

// A kind with a Source but no Handler is purely informational: its click
// just dismisses.
var (
	sources  = map[string]func() []Notice{}
	handlers = map[string]func(n Notice, client string) error{}
)

// Source wires a kind's lister, which derives the kind's current notices
// from its own state. Duplicate kinds panic.
func Source(kind string, list func() []Notice) {
	if _, ok := sources[kind]; ok {
		panic("statusbox: duplicate Source for kind " + kind)
	}
	sources[kind] = list
}

// Handler wires a kind's click handler, called with the freshly derived
// notice under the click. Duplicate kinds panic.
func Handler(kind string, handle func(n Notice, client string) error) {
	if _, ok := handlers[kind]; ok {
		panic("statusbox: duplicate Handler for kind " + kind)
	}
	handlers[kind] = handle
}

// state is the box's cache entry: the displayed notice (what a click acts
// on) and the dismissals, keyed by kind+id.
type state struct {
	Display   *Notice              `json:"display,omitempty"`
	Dismissed map[string]time.Time `json:"dismissed,omitempty"`
}

// collect gathers every source's notices, newest first, stamping Kind from
// the registered key so dismissal and dispatch never depend on a source
// filling it in.
func collect() []Notice {
	var notices []Notice
	for kind, list := range sources {
		for _, n := range list() {
			n.Kind = kind
			notices = append(notices, n)
		}
	}
	sort.Slice(notices, func(i, j int) bool {
		if !notices[i].Since.Equal(notices[j].Since) {
			return notices[i].Since.After(notices[j].Since)
		}
		if notices[i].ID != notices[j].ID {
			return notices[i].ID < notices[j].ID
		}
		return notices[i].Kind < notices[j].Kind
	})
	return notices
}

// renderBox renders the multibox and prunes the dismissal store: entries for
// gone notices or claims renewed after the dismissal are dropped, so
// dismissals self-invalidate without timers. disp is the displayed notice,
// nil outside notification state.
func renderBox(notices []Notice, dismissed map[string]time.Time) (box string, disp *Notice, keep map[string]time.Time) {
	keep = map[string]time.Time{}
	var undismissed []Notice
	for _, n := range notices {
		if at, ok := dismissed[n.key()]; ok && !n.Since.After(at) {
			keep[n.key()] = at
			continue
		}
		undismissed = append(undismissed, n)
	}
	if len(undismissed) > 0 {
		head := undismissed[0]
		text := fmt.Sprintf("✻ %s %s", escapeStatus(truncateLabel(head.Label)), head.Verb)
		if n := len(undismissed) - 1; n > 0 {
			text += fmt.Sprintf(" +%d", n)
		}
		box = fmt.Sprintf("#[fg=yellow]#[range=user|%s]%s#[norange] │#[range=user|%s] ✕ #[norange]#[fg=default]",
			rangeContent, text, rangeX)
		return box, &head, keep
	}
	if len(notices) > 0 {
		box = fmt.Sprintf("#[fg=colour245]✻ %s #[fg=default]", summarize(notices))
	}
	return box, nil, keep
}

// summarize counts notices per verb, e.g. "2 need input · 1 needs review".
func summarize(notices []Notice) string {
	counts := map[string]int{}
	plurals := map[string]string{}
	for _, n := range notices {
		counts[n.Verb]++
		if n.Plural != "" {
			plurals[n.Verb] = n.Plural
		}
	}
	verbs := make([]string, 0, len(counts))
	for v := range counts {
		verbs = append(verbs, v)
	}
	sort.Strings(verbs)
	parts := make([]string, 0, len(verbs))
	for _, v := range verbs {
		verb := v
		if counts[v] > 1 && plurals[v] != "" {
			verb = plurals[v]
		}
		parts = append(parts, fmt.Sprintf("%d %s", counts[v], verb))
	}
	return strings.Join(parts, " · ")
}

// maxLabel caps the notification label so a long label can't push the
// dismiss target past status-right-length.
const maxLabel = 24

// truncateLabel shortens the name part of "name:suffix", keeping the suffix
// — it's the jump-target hint.
func truncateLabel(label string) string {
	if utf8.RuneCountInString(label) <= maxLabel {
		return label
	}
	name, win := label, ""
	if i := strings.LastIndex(label, ":"); i >= 0 {
		name, win = label[:i], label[i:]
	}
	keep := max(maxLabel-utf8.RuneCountInString(win)-1, 1)
	if r := []rune(name); len(r) > keep {
		name = string(r[:keep])
	}
	return name + "…" + win
}

// escapeStatus doubles '#' so free-text labels never expand as formats or
// styles when the status line evaluates the box.
func escapeStatus(s string) string {
	return strings.ReplaceAll(s, "#", "##")
}

// Publish recomputes the multibox, publishes it, and refreshes every
// client's status line. Best-effort throughout: the box is decoration, so
// callers (hooks especially) never fail because a status line didn't update.
func Publish() {
	var st state
	cachefile.Read(stateFile, &st)
	box, disp, keep := renderBox(collect(), st.Dismissed)
	cachefile.Write(stateFile, state{Display: disp, Dismissed: keep})
	tmuxctl.SetGlobalOption(boxOption, box)
	for _, client := range tmuxctl.ListClients() {
		tmuxctl.RefreshStatus(client)
	}
}

// RunClick handles `jmux statusline click <range> [client]`, the mouse
// binding's dispatch: rangeContent runs the displayed notice's handler; both
// ranges dismiss it — the notification was seen, and a renewed claim
// reinstates it. Stale clicks republish to self-heal.
func RunClick(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: jmux statusline click <range> [client]")
	}
	var client string
	if len(args) > 1 {
		client = args[1]
	}
	var st state
	cachefile.Read(stateFile, &st)
	if st.Display == nil || st.Display.ID == "" {
		Publish()
		return nil
	}
	switch args[0] {
	case rangeContent:
		// Dispatch to the current notice, not the snapshot: a notice that
		// vanished since the render just republishes instead of acting stale.
		n, ok := find(collect(), st.Display.key())
		if !ok {
			Publish()
			return nil
		}
		if handle := handlers[n.Kind]; handle != nil {
			if err := handle(n, client); err != nil {
				Publish()
				return err
			}
		}
		dismiss(st)
		return nil
	case rangeX:
		dismiss(st)
		return nil
	}
	return fmt.Errorf("unknown status range %q", args[0])
}

func find(notices []Notice, key string) (Notice, bool) {
	for _, n := range notices {
		if n.key() == key {
			return n, true
		}
	}
	return Notice{}, false
}

// dismiss marks the displayed notice seen and republishes.
func dismiss(st state) {
	if st.Dismissed == nil {
		st.Dismissed = map[string]time.Time{}
	}
	st.Dismissed[st.Display.key()] = time.Now()
	cachefile.Write(stateFile, st)
	Publish()
}
