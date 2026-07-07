// Package tag is a durable, path-keyed tag store: any package tags a
// workspace (Set) and the overview renders every tag (All). Tags carry
// semantic data, not rendered text — each kind's owner registers a renderer
// (Register) that decodes the data at read time, so display changes never go
// stale in the store. Tags live in jmux's cache, so losing the store only
// drops annotations, never a workspace.
package tag

import (
	"encoding/json"
	"sort"
	"strings"

	"jmux/internal/cachefile"
	"jmux/internal/repo"
)

const storeFile = "tags.json"

// Tag is what a source attaches to a workspace. Pane, when set, is the tmux
// pane the source runs in — the overview uses it to label the tag with its
// window and drop it when the pane dies. Data is whatever the kind's renderer
// decodes. Construct via New.
type Tag struct {
	Kind string          `json:"kind"`
	Pane string          `json:"pane,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

// New builds a tag of the given kind, marshaling payload into Data.
func New(kind, pane string, payload any) Tag {
	data, _ := json.Marshal(payload)
	return Tag{Kind: kind, Pane: pane, Data: data}
}

// Color names a tag's display colour (an ANSI SGR code). The zero value
// renders plain.
type Color string

const (
	Green  Color = "32"
	Yellow Color = "33"
	Cyan   Color = "36"
	Gray   Color = "90"
)

// renderers maps a kind to its registered describe fn.
var renderers = map[string]func(json.RawMessage) (string, Color){}

// Register wires a kind's renderer: decode the tag's data and return its
// display text and colour (empty text hides the tag). Producers expose an
// idempotent RegisterTag that main calls; duplicate kinds panic.
func Register(kind string, describe func(data json.RawMessage) (string, Color)) {
	if renderers[kind] != nil {
		panic("tag: duplicate Register for kind " + kind)
	}
	renderers[kind] = describe
}

// describe renders the tag via its kind's registered renderer. Kinds without
// one (unknown, unlinked, or legacy pre-semantic entries) come back empty and
// are skipped by All.
func (t Tag) describe() (string, Color) {
	describe := renderers[t.Kind]
	if describe == nil {
		return "", ""
	}
	return describe(t.Data)
}

// Text returns the tag's display text without colour.
func (t Tag) Text() string {
	text, _ := t.describe()
	return text
}

// Render wraps the tag's text in its colour's ANSI code for an --ansi fzf
// list.
func (t Tag) Render() string {
	text, c := t.describe()
	if text == "" || c == "" {
		return text
	}
	return "\x1b[" + string(c) + "m" + text + "\x1b[0m"
}

// store maps a resolved path to its tags keyed by namespace, so a re-Set from
// the same source replaces rather than stacks.
type store map[string]map[string]Tag

// Set tags path with t under namespace ns, replacing ns's prior tag and
// pruning entries whose directory is gone or that predate the semantic store
// (kindless). Best-effort: a write failure just means the tag won't show.
func Set(path, ns string, t Tag) {
	s := store{}
	cachefile.Read(storeFile, &s)
	for p, byNS := range s {
		if !repo.IsDir(p) {
			delete(s, p)
			continue
		}
		for n, old := range byNS {
			if old.Kind == "" {
				delete(byNS, n)
			}
		}
		if len(byNS) == 0 {
			delete(s, p)
		}
	}
	key := repo.Resolve(path)
	if s[key] == nil {
		s[key] = map[string]Tag{}
	}
	s[key][ns] = t
	cachefile.Write(storeFile, s)
}

// Unset removes the ns tag from path, dropping the path entry when it was
// the last one.
func Unset(path, ns string) {
	s := store{}
	cachefile.Read(storeFile, &s)
	key := repo.Resolve(path)
	if s[key] == nil {
		return
	}
	delete(s[key], ns)
	if len(s[key]) == 0 {
		delete(s, key)
	}
	cachefile.Write(storeFile, s)
}

// UnsetPrefix removes every tag on path whose namespace starts with prefix,
// dropping the path entry when nothing remains.
func UnsetPrefix(path, prefix string) {
	s := store{}
	cachefile.Read(storeFile, &s)
	key := repo.Resolve(path)
	if s[key] == nil {
		return
	}
	for ns := range s[key] {
		if strings.HasPrefix(ns, prefix) {
			delete(s[key], ns)
		}
	}
	if len(s[key]) == 0 {
		delete(s, key)
	}
	cachefile.Write(storeFile, s)
}

// All maps every resolved path to its tags, ordered by namespace for stable
// display. Tags no renderer can describe (legacy, unknown, or unlinked kinds)
// are skipped rather than rendered blank.
func All() map[string][]Tag {
	s := store{}
	cachefile.Read(storeFile, &s)
	out := make(map[string][]Tag, len(s))
	for path, byNS := range s {
		names := make([]string, 0, len(byNS))
		for ns := range byNS {
			names = append(names, ns)
		}
		sort.Strings(names)
		tags := make([]Tag, 0, len(byNS))
		for _, ns := range names {
			if byNS[ns].Text() != "" {
				tags = append(tags, byNS[ns])
			}
		}
		if len(tags) > 0 {
			out[path] = tags
		}
	}
	return out
}
