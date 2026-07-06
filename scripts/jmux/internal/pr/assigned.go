package pr

import (
	"fmt"
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
	if !ensureGH() {
		return
	}

	self := shellQuote(fzfutil.Self())
	sel, err := pickPR("prs> ", self+" pr items", self+" pr items --refresh")
	if err != nil || sel == "" {
		return
	}

	slug, num, ok := parseRepoNumber(sel)
	if !ok {
		return
	}
	if err := reviewSelection(slug, num, sel); err != nil {
		notify.Error(err.Error())
	}
}

// reviewSelection maps a picked cross-repo PR to its local clone and reviews it.
func reviewSelection(slug string, num int, sel string) error {
	bareRoot := findLocalRepo(slug)
	if bareRoot == "" {
		return fmt.Errorf("no local clone of %s", slug)
	}
	headRef, err := resolveHeadRef(sel, slug, num)
	if err != nil {
		return err
	}
	return review(bareRoot, ghctl.PR{Number: num, HeadRefName: headRef, BaseRefName: rowBaseRef(sel)})
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
// ("owner/repo#12").
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
