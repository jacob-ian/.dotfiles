package pr

import (
	"fmt"
	"os"
	"strings"
	"time"

	"jmux/internal/cachefile"
	"jmux/internal/ghctl"
	"jmux/internal/gitctl"
	"jmux/internal/notify"
)

// cacheTTL bounds how long `jmux pr` serves the cached list before refetching on
// open. ctrl-r in the picker forces a refresh regardless of age.
const cacheTTL = 5 * time.Minute

const cacheFile = "prs.json"

// orgEnv is a comma-separated org whitelist for `jmux pr`; unset means all orgs.
const orgEnv = "JMUX_PR_ORGS"

// allowedOrgs parses orgEnv into owner names; nil means no restriction.
func allowedOrgs() []string {
	raw := strings.TrimSpace(os.Getenv(orgEnv))
	if raw == "" {
		return nil
	}
	var orgs []string
	for o := range strings.SplitSeq(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			orgs = append(orgs, o)
		}
	}
	return orgs
}

type prCache struct {
	FetchedAt time.Time            `json:"fetched_at"`
	Results   []ghctl.SearchResult `json:"results"`
}

// readCache returns the cached results and their age. ok is false when no
// readable, well-formed cache exists.
func readCache() (results []ghctl.SearchResult, age time.Duration, ok bool) {
	var c prCache
	if !cachefile.Read(cacheFile, &c) {
		return nil, 0, false
	}
	return c.Results, time.Since(c.FetchedAt), true
}

// loadResults returns the PR list, fetching from GitHub and refreshing the cache
// when forced, when no cache exists, or when it's older than cacheTTL.
func loadResults(refresh bool) ([]ghctl.SearchResult, error) {
	if !refresh {
		if results, age, ok := readCache(); ok && age < cacheTTL {
			return results, nil
		}
	}
	results, err := ghctl.SearchMyPRs(allowedOrgs())
	if err != nil {
		return nil, err
	}
	cachefile.Write(cacheFile, prCache{FetchedAt: time.Now(), Results: results})
	return results, nil
}

// formatRows renders the picker rows for results.
func formatRows(results []ghctl.SearchResult) []string {
	items := make([]string, len(results))
	for i, r := range results {
		items[i] = formatItemsRow(r.Repository.NameWithOwner, r.Number, r.IsDraft, r.Title, r.Author.Login, r.HeadRefName, r.BaseRefName)
	}
	return items
}

// RunItems handles `jmux pr items [--refresh]`: print the picker rows to stdout
// for fzf's reload binding. On a refresh error it falls back to the cached rows
// so the picker keeps its current list rather than clearing.
func RunItems(args []string) {
	if len(args) >= 2 && args[0] == "--repo" {
		if err := printRepoItems(args[1]); err != nil {
			notify.Errorf("list PRs: %s", gitctl.CleanErr(err))
		}
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

// printRepoItems prints one repo's open-PR rows to stdout for the per-repo
// picker's reload binding.
func printRepoItems(slug string) error {
	prs, err := ghctl.ListPRs(slug)
	if err != nil {
		return err
	}
	for _, p := range prs {
		fmt.Println(formatItemsRow(slug, p.Number, p.IsDraft, p.Title, p.Author.Login, p.HeadRefName, p.BaseRefName))
	}
	return nil
}
