// Package statusbox renders the tmux status-line multibox: the newest
// undismissed notice as a clickable notification, a summary once all are dismissed.
package statusbox

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"jmux/internal/cachefile"
	"jmux/internal/tmuxctl"
)

// tmux caps range names at 15 bytes, so the ranges are fixed names and a
// click resolves through the persisted display entry.
const (
	boxOption    = "@jmux_statusbox"
	rangeContent = "jmux-content"
	rangeX       = "jmux-x"
)

const stateFile = "statusbox.json"

// Notice is one item claiming attention; behaviour lives in the kind's Handler.
type Notice struct {
	Kind   string    `json:"kind"` // stamped by collect, never by sources
	ID     string    `json:"id"`   // Kind+ID is the notice's key
	Label  string    `json:"label,omitempty"`
	Verb   string    `json:"verb,omitempty"`   // e.g. "needs input"
	Plural string    `json:"plural,omitempty"` // empty reuses Verb
	Since  time.Time `json:"since,omitzero"`   // zero never renews, so dismissals stick
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

// Source registers a kind's lister; duplicate kinds panic.
func Source(kind string, list func() []Notice) {
	if _, ok := sources[kind]; ok {
		panic("statusbox: duplicate Source for kind " + kind)
	}
	sources[kind] = list
}

// Handler registers a kind's click handler; duplicate kinds panic.
func Handler(kind string, handle func(n Notice, client string) error) {
	if _, ok := handlers[kind]; ok {
		panic("statusbox: duplicate Handler for kind " + kind)
	}
	handlers[kind] = handle
}

type state struct {
	Display   *Notice              `json:"display,omitempty"`
	Dismissed map[string]time.Time `json:"dismissed,omitempty"`
}

// collect stamps Kind from the registry key so dispatch never depends on a
// source filling it in.
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

// renderBox renders the box and prunes dismissals for notices gone or renewed
// since, so they self-invalidate without timers.
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

// maxLabel keeps a long label from pushing the dismiss target past status-right-length.
const maxLabel = 24

func truncateLabel(label string) string {
	r := []rune(label)
	if len(r) <= maxLabel {
		return label
	}
	return string(r[:maxLabel-1]) + "…"
}

// escapeStatus doubles '#' so labels never expand as tmux formats.
func escapeStatus(s string) string {
	return strings.ReplaceAll(s, "#", "##")
}

// Publish rerenders the box and refreshes every client. Best-effort: hooks
// must never fail because a status line didn't update.
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

// RunClick handles `jmux statusline click <range> [client]`: rangeContent
// runs the displayed notice's handler, both ranges dismiss it.
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
		// Dispatch to the current notice, not the snapshot, so a vanished one
		// republishes instead of acting stale.
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

func dismiss(st state) {
	if st.Dismissed == nil {
		st.Dismissed = map[string]time.Time{}
	}
	st.Dismissed[st.Display.key()] = time.Now()
	cachefile.Write(stateFile, st)
	Publish()
}
