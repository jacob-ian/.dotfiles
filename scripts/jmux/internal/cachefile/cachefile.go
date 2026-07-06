// Package cachefile reads and writes JSON files under the user cache dir
// (<cache>/jmux). Writes go through temp-then-rename so a concurrent jmux
// process never reads a half-written file.
package cachefile

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func path(name string) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jmux", name), nil
}

// Read unmarshals the named cache file into v, reporting whether a readable,
// well-formed file was found.
func Read(name string, v any) bool {
	p, err := path(name)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

// Write marshals v to the named cache file. Best-effort: on failure the cache
// is simply stale or absent next read.
func Write(name string, v any) {
	p, err := path(name)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), name+".tmp-*")
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
	if err := os.Rename(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
	}
}
