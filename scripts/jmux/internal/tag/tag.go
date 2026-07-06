// Package tag is a durable, path-keyed badge store: any package tags a
// workspace (Set) and the overview renders every tag (All). Tags live in
// jmux's cache, so losing the store only drops annotations, never a workspace.
package tag

import (
	"sort"

	"jmux/internal/cachefile"
	"jmux/internal/repo"
)

const storeFile = "tags.json"

// Color names a badge's display colour. The zero value renders plain.
type Color string

const (
	Default Color = ""
	Red     Color = "red"
	Green   Color = "green"
	Yellow  Color = "yellow"
	Blue    Color = "blue"
	Magenta Color = "magenta"
	Cyan    Color = "cyan"
	Gray    Color = "gray"
)

var ansiCodes = map[Color]string{
	Red:     "31",
	Green:   "32",
	Yellow:  "33",
	Blue:    "34",
	Magenta: "35",
	Cyan:    "36",
	Gray:    "90",
}

// Badge is a single workspace tag: display text and the colour to show it in.
type Badge struct {
	Text  string `json:"text"`
	Color Color  `json:"color,omitempty"`
}

// Render wraps the text in its colour's ANSI code for an --ansi fzf list; an
// empty or unknown colour renders plain.
func (b Badge) Render() string {
	code := ansiCodes[b.Color]
	if code == "" {
		return b.Text
	}
	return "\x1b[" + code + "m" + b.Text + "\x1b[0m"
}

// store maps a resolved path to its badges keyed by namespace, so a re-Set from
// the same source replaces rather than stacks.
type store map[string]map[string]Badge

// Set tags path with badge b under namespace ns, replacing ns's prior badge and
// pruning entries whose directory is gone. Best-effort: a write failure just
// means the tag won't show.
func Set(path, ns string, b Badge) {
	s := store{}
	cachefile.Read(storeFile, &s)
	for p := range s {
		if !repo.IsDir(p) {
			delete(s, p)
		}
	}
	key := repo.Resolve(path)
	if s[key] == nil {
		s[key] = map[string]Badge{}
	}
	s[key][ns] = b
	cachefile.Write(storeFile, s)
}

// Unset removes the ns badge from path, dropping the path entry when it was
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

// All maps every resolved path to its badges, ordered by namespace for stable
// display.
func All() map[string][]Badge {
	s := store{}
	cachefile.Read(storeFile, &s)
	out := make(map[string][]Badge, len(s))
	for path, byNS := range s {
		names := make([]string, 0, len(byNS))
		for ns := range byNS {
			names = append(names, ns)
		}
		sort.Strings(names)
		badges := make([]Badge, 0, len(byNS))
		for _, ns := range names {
			badges = append(badges, byNS[ns])
		}
		out[path] = badges
	}
	return out
}
