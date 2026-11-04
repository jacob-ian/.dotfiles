// Package tag is a durable, path-keyed tag store. Tags carry data, not
// rendered text: each kind registers a renderer that decodes at read time, so
// display changes never go stale in the store.
package tag

import (
	"encoding/json"
	"sort"
	"strings"

	"jmux/internal/cachefile"
	"jmux/internal/repo"
)

const storeFile = "tags.json"

// Tag is what a source attaches to a workspace. Pane is the tmux pane the
// source runs in, used to label the tag and drop it when the pane dies.
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

// Color is an ANSI SGR code; the zero value renders plain.
type Color string

const (
	Green  Color = "32"
	Yellow Color = "33"
	Cyan   Color = "36"
	Gray   Color = "90"
)

var renderers = map[string]func(json.RawMessage) (string, Color){}

// Register wires a kind's renderer (empty text hides the tag); duplicate kinds panic.
func Register(kind string, describe func(data json.RawMessage) (string, Color)) {
	if renderers[kind] != nil {
		panic("tag: duplicate Register for kind " + kind)
	}
	renderers[kind] = describe
}

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

// Render wraps the tag's text in its colour's ANSI code.
func (t Tag) Render() string {
	text, c := t.describe()
	if text == "" || c == "" {
		return text
	}
	return "\x1b[" + string(c) + "m" + text + "\x1b[0m"
}

type store map[string]map[string]Tag

// Set replaces ns's prior tag on path and prunes entries whose directory is
// gone or that predate the semantic store.
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

// Get returns path's tag under namespace ns, when present.
func Get(path, ns string) (Tag, bool) {
	s := store{}
	cachefile.Read(storeFile, &s)
	t, ok := s[repo.Resolve(path)][ns]
	return t, ok
}

// Unset removes path's ns tag.
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

// UnsetPrefix removes every tag on path whose namespace starts with prefix.
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

// All maps every resolved path to its tags, ordered by namespace; tags
// without a renderer are skipped.
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
