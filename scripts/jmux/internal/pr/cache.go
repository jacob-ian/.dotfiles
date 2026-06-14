package pr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"jmux/internal/ghctl"
	"jmux/internal/gitctl"
	"jmux/internal/notify"
)

// cacheTTL bounds how long `jmux pr` serves the cached list before refetching on
// open. ctrl-r in the picker forces a refresh regardless of age.
const cacheTTL = 5 * time.Minute

type prCache struct {
	FetchedAt time.Time            `json:"fetched_at"`
	Results   []ghctl.SearchResult `json:"results"`
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jmux", "prs.json"), nil
}

// readCache returns the cached results and their age. ok is false when no
// readable, well-formed cache exists.
func readCache() (results []ghctl.SearchResult, age time.Duration, ok bool) {
	path, err := cachePath()
	if err != nil {
		return nil, 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false
	}
	var c prCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, 0, false
	}
	return c.Results, time.Since(c.FetchedAt), true
}

// writeCache persists results stamped now. Best-effort: a failure just means the
// next open refetches. The temp-then-rename keeps a concurrent jmux process from
// reading a half-written file.
func writeCache(results []ghctl.SearchResult) {
	path, err := cachePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(prCache{FetchedAt: time.Now(), Results: results})
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "prs-*.json")
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

// loadResults returns the PR list, fetching from GitHub and refreshing the cache
// when forced, when no cache exists, or when it's older than cacheTTL.
func loadResults(refresh bool) ([]ghctl.SearchResult, error) {
	if !refresh {
		if results, age, ok := readCache(); ok && age < cacheTTL {
			return results, nil
		}
	}
	results, err := ghctl.SearchMyPRs()
	if err != nil {
		return nil, err
	}
	writeCache(results)
	return results, nil
}

// formatRows renders the picker rows for results.
func formatRows(results []ghctl.SearchResult) []string {
	items := make([]string, len(results))
	for i, r := range results {
		items[i] = formatRow(r.Repository.NameWithOwner, r.Number, r.IsDraft, r.Title, r.Author.Login)
	}
	return items
}

// RunItems handles `jmux pr items [--refresh]`: print the picker rows to stdout
// for fzf's reload binding. On a refresh error it falls back to the cached rows
// so the picker keeps its current list rather than clearing.
func RunItems(args []string) {
	if len(args) >= 2 && args[0] == "--repo" {
		runRepoItems(args[1])
		return
	}
	refresh := len(args) > 0 && args[0] == "--refresh"
	results, err := loadResults(refresh)
	if err != nil {
		if cached, _, ok := readCache(); ok {
			results = cached
		}
		notify.Errorf("refresh PRs: %s", gitctl.CleanErr(err))
	}
	for _, row := range formatRows(results) {
		fmt.Println(row)
	}
}

// runRepoItems handles `jmux pr items --repo <slug>`: print one repo's open-PR
// rows to stdout for the per-repo picker's reload binding.
func runRepoItems(slug string) {
	prs, err := ghctl.ListPRs(slug)
	if err != nil {
		notify.Errorf("list PRs: %s", gitctl.CleanErr(err))
		return
	}
	for _, p := range prs {
		fmt.Println(formatRow(slug, p.Number, p.IsDraft, p.Title, p.Author.Login))
	}
}
