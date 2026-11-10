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

const cacheTTL = 5 * time.Minute

const cacheFile = "prs.json"

// orgEnv is a comma-separated org allowlist; unset means all orgs.
const orgEnv = "JMUX_PR_ORGS"

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

func readCache() (results []ghctl.SearchResult, age time.Duration, ok bool) {
	var c prCache
	if !cachefile.Read(cacheFile, &c) {
		return nil, 0, false
	}
	return c.Results, time.Since(c.FetchedAt), true
}

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

func formatRows(results []ghctl.SearchResult) []string {
	items := make([]string, len(results))
	for i, r := range results {
		items[i] = formatItemsRow(rowMarker(r.Author.Login), r.Repository.NameWithOwner, r.Number, r.IsDraft, r.Title, byline(r.Author.Login, r.Owner), r.HeadRefName, r.BaseRefName)
	}
	return items
}

// RunItems handles `jmux fzf pr items [--refresh|--repo <slug>]`, printing the
// picker rows for fzf's reload bindings. A failed refresh keeps the cached rows
// so the picker doesn't clear.
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

func printRepoItems(slug string) error {
	prs, err := ghctl.ListPRs(slug)
	if err != nil {
		return err
	}
	for _, row := range formatRows(prs) {
		fmt.Println(row)
	}
	return nil
}
