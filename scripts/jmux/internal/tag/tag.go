// Package tag is a durable, path-keyed badge store: any package tags a
// workspace (Set) and the overview renders every tag (All). Tags live in
// jmux's cache, so losing the store only drops annotations, never a workspace.
package tag

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

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

func storePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jmux", "tags.json"), nil
}

// store maps a resolved path to its badges keyed by namespace, so a re-Set from
// the same source replaces rather than stacks.
type store map[string]map[string]Badge

// load returns the on-disk store, or an empty one when none is readable.
func load() store {
	s := store{}
	path, err := storePath()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

// Set tags path with badge b under namespace ns, replacing ns's prior badge and
// pruning entries whose directory is gone. Best-effort: a write failure just
// means the tag won't show.
func Set(path, ns string, b Badge) {
	s := load()
	for p := range s {
		if !isDir(p) {
			delete(s, p)
		}
	}
	key := resolve(path)
	if s[key] == nil {
		s[key] = map[string]Badge{}
	}
	s[key][ns] = b
	write(s)
}

// All maps every resolved path to its badges, ordered by namespace for stable
// display.
func All() map[string][]Badge {
	s := load()
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

// write persists s via a temp-then-rename so a concurrent jmux process never
// reads a half-written file.
func write(s store) {
	path, err := storePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "tags-*.json")
	if err != nil {
		return
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
	}
}

func resolve(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
