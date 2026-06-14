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

	self, err := os.Executable()
	if err != nil {
		self = "jmux"
	}

	// start:reload loads the list async behind fzf's spinner so the open doesn't
	// block on gh; the load event swaps the "loading…" prompt back.
	sel, err := fzfutil.Pick(nil, fzfutil.Options{
		Prompt: "loading… ",
		Header: "enter: review · ctrl-r: refresh · ctrl-/: toggle preview",
		Bindings: []string{
			"ctrl-/:toggle-preview",
			fmt.Sprintf("start:reload(%s pr items)", shellQuote(self)),
			"load:change-prompt(prs> )",
			fmt.Sprintf("ctrl-r:change-prompt(refreshing… )+reload(%s pr items --refresh)", shellQuote(self)),
		},
		Preview:       fmt.Sprintf("%s pr preview {}", shellQuote(self)),
		PreviewWindow: "right:60%:wrap",
		Delimiter:     "\t",
		WithNth:       "1",
	})
	if err != nil || sel == "" {
		return
	}

	slug, num, ok := parseRepoNumber(sel)
	if !ok {
		return
	}
	reviewSelection(slug, num, rowHeadRef(sel))
}

// reviewSelection maps a picked cross-repo PR to its local clone and reviews it.
// headRef comes from the row's hidden field, falling back to a lookup when absent.
func reviewSelection(slug string, num int, headRef string) {
	bareRoot := findLocalRepo(slug)
	if bareRoot == "" {
		notify.Errorf("No local clone of %s", slug)
		return
	}
	if headRef == "" {
		var err error
		if headRef, err = ghctl.HeadRef(slug, num); err != nil || headRef == "" {
			notify.Errorf("resolve branch for %s#%d: %s", slug, num, gitctl.CleanErr(err))
			return
		}
	}
	Review(bareRoot, ghctl.PR{Number: num, HeadRefName: headRef})
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
