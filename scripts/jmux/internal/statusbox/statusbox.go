// Package statusbox renders the tmux status-line multibox: the newest
// attention-claiming tag as a clickable notification, a waiting count once
// everything is dismissed, nothing when nothing waits. Any tag kind
// participates by registering a tag.Attention fn; the box itself is
// kind-agnostic. State is published to a global tmux user option that
// status-right references, so one render reaches every session.
package statusbox

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"jmux/internal/cachefile"
	"jmux/internal/tag"
	"jmux/internal/tmuxctl"
)

// boxOption is the tmux user option the status line references via
// #{E:@jmux_statusbox}; rangeGo/rangeDismiss are the user-range names the
// MouseDown1Status binding dispatches on (tmux caps range arguments at 15
// bytes). Only the displayed notification carries ranges, so the names are
// fixed and the click resolves through the persisted display item.
const (
	boxOption    = "@jmux_statusbox"
	rangeGo      = "jmux-go"
	rangeDismiss = "jmux-x"
)

const stateFile = "statusbox.json"

// state is the box's cache entry: the item the notification currently points
// at (what a click acts on) and the dismissals, keyed by kind+pane.
type state struct {
	Display   *displayed           `json:"display,omitempty"`
	Dismissed map[string]time.Time `json:"dismissed,omitempty"`
}

type displayed struct {
	Kind string `json:"kind"`
	Pane string `json:"pane"`
}

// item is one live pane's attention claim: where a click jumps, labelled
// "session:window-index".
type item struct {
	kind  string
	pane  string
	label string
	verb  string
	since time.Time
}

func (it item) key() string {
	return it.kind + " " + it.pane
}

// buildItems flattens the tag store into attention claims, dropping tags with
// no pane to jump to (unset or dead) and deduping each kind's tags per pane
// to the most recent — a newer quiet tag supersedes a stale claim from a
// predecessor in the same pane. Newest first.
func buildItems(tags map[string][]tag.Tag, labels map[string]string) []item {
	newest := map[string]item{}
	for _, ts := range tags {
		for _, t := range ts {
			label := labels[t.Pane]
			if t.Pane == "" || label == "" {
				continue
			}
			a := t.Attention()
			if a.Verb == "" && a.Since.IsZero() {
				continue
			}
			it := item{kind: t.Kind, pane: t.Pane, label: label, verb: a.Verb, since: a.Since}
			if prev, ok := newest[it.key()]; !ok || it.since.After(prev.since) {
				newest[it.key()] = it
			}
		}
	}
	items := make([]item, 0, len(newest))
	for _, it := range newest {
		if it.verb != "" {
			items = append(items, it)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].since.Equal(items[j].since) {
			return items[i].since.After(items[j].since)
		}
		if items[i].pane != items[j].pane {
			return items[i].pane < items[j].pane
		}
		return items[i].kind < items[j].kind
	})
	return items
}

// renderBox renders the multibox and prunes the dismissal store: entries for
// gone panes, quieted tags, or claims renewed after the dismissal are
// dropped, so dismissals self-invalidate without timers. The returned display
// is the item the notification's click targets, nil outside notification
// state.
func renderBox(items []item, dismissed map[string]time.Time) (box string, disp *displayed, keep map[string]time.Time) {
	keep = map[string]time.Time{}
	var undismissed []item
	for _, it := range items {
		if at, ok := dismissed[it.key()]; ok && !it.since.After(at) {
			keep[it.key()] = at
			continue
		}
		undismissed = append(undismissed, it)
	}
	if len(undismissed) > 0 {
		head := undismissed[0]
		text := fmt.Sprintf("✻ %s %s", escapeStatus(truncateLabel(head.label)), head.verb)
		if n := len(undismissed) - 1; n > 0 {
			text += fmt.Sprintf(" +%d", n)
		}
		box = fmt.Sprintf("#[fg=yellow]#[range=user|%s]%s#[norange] │#[range=user|%s] ✕ #[norange]#[fg=default]",
			rangeGo, text, rangeDismiss)
		return box, &displayed{Kind: head.kind, Pane: head.pane}, keep
	}
	if len(items) > 0 {
		box = fmt.Sprintf("#[fg=colour245]✻ %d waiting #[fg=default]", len(items))
	}
	return box, nil, keep
}

// maxLabel caps the notification label so a long session name can't push the
// dismiss target past status-right-length.
const maxLabel = 24

// truncateLabel shortens the session-name part of "session:window" to fit
// maxLabel runes, keeping the window index — it's the jump-target hint.
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

// escapeStatus doubles '#' so free-text session names never expand as
// formats or styles when the status line evaluates the box.
func escapeStatus(s string) string {
	return strings.ReplaceAll(s, "#", "##")
}

// Publish recomputes the multibox from the tag store and live panes, writes
// the pruned state back, publishes the rendered box, and refreshes every
// client's status line. Best-effort throughout: the box is decoration, so
// callers (hooks especially) never fail because a status line didn't update.
func Publish() {
	var st state
	cachefile.Read(stateFile, &st)
	items := buildItems(tag.All(), tmuxctl.PaneLabels())
	box, disp, keep := renderBox(items, st.Dismissed)
	cachefile.Write(stateFile, state{Display: disp, Dismissed: keep})
	tmuxctl.SetGlobalOption(boxOption, box)
	for _, client := range tmuxctl.ListClients() {
		tmuxctl.RefreshStatus(client)
	}
}

// RunClick handles `jmux statusline click <range> [client]`, the status-line
// mouse binding's dispatch: rangeGo jumps the clicking client to the
// displayed item's pane, and both ranges dismiss it — a click means the
// notification was seen, and a renewed claim reinstates it. A click that
// races the display going stale republishes to self-heal.
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
	if st.Display == nil {
		Publish()
		return nil
	}
	switch args[0] {
	case rangeGo:
		target := tmuxctl.PaneTarget(st.Display.Pane)
		if target == "" {
			Publish()
			return errors.New("pane is gone")
		}
		if err := tmuxctl.SwitchClientTo(client, target); err != nil {
			return fmt.Errorf("switching to %s: %w", target, err)
		}
		if err := tmuxctl.SelectPane(st.Display.Pane); err != nil {
			return err
		}
		dismiss(st)
		return nil
	case rangeDismiss:
		dismiss(st)
		return nil
	}
	return fmt.Errorf("unknown status range %q", args[0])
}

// dismiss records the displayed item as seen and republishes, advancing the
// box to the next queued claim or the summary.
func dismiss(st state) {
	if st.Dismissed == nil {
		st.Dismissed = map[string]time.Time{}
	}
	st.Dismissed[st.Display.Kind+" "+st.Display.Pane] = time.Now()
	cachefile.Write(stateFile, st)
	Publish()
}
