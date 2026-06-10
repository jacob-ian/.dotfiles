package pr

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"jmux/internal/fzfutil"
	"jmux/internal/ghctl"
	"jmux/internal/gitctl"
	"jmux/internal/notify"
	"jmux/internal/repo"
)

// RunAssigned handles `jmux pr`: the cross-repo review queue of PRs that request
// your review or are assigned to you. Unlike `jmux pr <dir>` it isn't tied to
// one repo — it searches every repo, then maps the chosen PR back to a local
// clone to check out as usual.
func RunAssigned() {
	if !ghctl.Available() {
		notify.Error("gh CLI not found — install the GitHub CLI to review PRs")
		return
	}

	results, err := ghctl.SearchAssignedPRs()
	if err != nil {
		notify.Errorf("gh search prs: %s", gitctl.CleanErr(err))
		return
	}
	if len(results) == 0 {
		notify.Info("No PRs assigned to or awaiting review from you")
		return
	}

	items := make([]string, len(results))
	byLine := make(map[string]ghctl.SearchResult, len(results))
	for i, r := range results {
		items[i] = formatRow(r.Repository.NameWithOwner, r.Number, r.IsDraft, r.Title, r.Author.Login)
		byLine[items[i]] = r
	}

	self, err := os.Executable()
	if err != nil {
		self = "jmux"
	}

	sel, err := fzfutil.Pick(items, fzfutil.Options{
		Prompt:        "assigned> ",
		Header:        "enter: review · ctrl-/: toggle preview",
		Bindings:      []string{"ctrl-/:toggle-preview"},
		Preview:       fmt.Sprintf("%s pr preview {}", shellQuote(self)),
		PreviewWindow: "right:60%:wrap",
	})
	if err != nil || sel == "" {
		return
	}

	r, ok := byLine[sel]
	if !ok {
		return
	}
	reviewSearchResult(r)
}

// reviewSearchResult maps a cross-repo PR to its local clone, resolves the head
// branch search omits, and hands off to the shared review flow.
func reviewSearchResult(r ghctl.SearchResult) {
	bareRoot := findLocalRepo(r.Repository.NameWithOwner)
	if bareRoot == "" {
		notify.Errorf("No local clone of %s", r.Repository.NameWithOwner)
		return
	}
	headRef, err := ghctl.HeadRef(r.Repository.NameWithOwner, r.Number)
	if err != nil || headRef == "" {
		notify.Errorf("resolve branch for %s#%d: %s", r.Repository.NameWithOwner, r.Number, gitctl.CleanErr(err))
		return
	}
	Review(bareRoot, ghctl.PR{Number: r.Number, HeadRefName: headRef})
}

// findLocalRepo returns the bare repo whose origin remote is nameWithOwner, or
// "" if no local clone matches.
func findLocalRepo(nameWithOwner string) string {
	for _, bare := range repo.BareRepos() {
		if strings.EqualFold(gitctl.RepoSlug(bare), nameWithOwner) {
			return bare
		}
	}
	return ""
}

// parseRepoNumber extracts "owner/repo" and the number from a row's first field
// ("owner/repo#12"). Used by the cross-repo preview.
func parseRepoNumber(line string) (string, int, bool) {
	field := strings.TrimSpace(line)
	if i := strings.IndexAny(field, " \t"); i >= 0 {
		field = field[:i]
	}
	h := strings.LastIndex(field, "#")
	if h <= 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(field[h+1:])
	if err != nil {
		return "", 0, false
	}
	return field[:h], n, true
}
