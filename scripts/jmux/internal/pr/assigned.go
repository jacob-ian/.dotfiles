package pr

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"jmux/internal/fzfutil"
	"jmux/internal/ghctl"
	"jmux/internal/gitctl"
	"jmux/internal/repo"
	"jmux/internal/tmuxctl"
)

// RunAssigned handles `jmux pr`: the cross-repo queue of PRs that request your
// review or are assigned to you, mapped back to a local clone on pick.
func RunAssigned(cfg repo.Config) error {
	if !ghctl.Available() {
		return errNoGH
	}

	self := tmuxctl.ShellQuote(fzfutil.Self())
	sel, err := pickPR("prs> ", self+" fzf pr items", self+" fzf pr items --refresh")
	if err != nil || sel == "" {
		return nil
	}

	slug, num, ok := parseRepoNumber(sel)
	if !ok {
		return nil
	}
	return reviewSelection(cfg, slug, num, sel)
}

func reviewSelection(cfg repo.Config, slug string, num int, sel string) error {
	bareRoot := findLocalRepo(cfg, slug)
	if bareRoot == "" {
		return fmt.Errorf("no local clone of %s", slug)
	}
	headRef, err := resolveHeadRef(sel, slug, num)
	if err != nil {
		return err
	}
	return review(bareRoot, ghctl.PR{Number: num, HeadRefName: headRef, BaseRefName: rowBaseRef(sel)})
}

// findLocalRepo returns the bare repo whose origin is nameWithOwner. Each check
// is a git call, so clones named after the repo are tried first.
func findLocalRepo(cfg repo.Config, nameWithOwner string) string {
	_, name, _ := strings.Cut(nameWithOwner, "/")
	bares := cfg.BareRepos()
	named := func(bare string) bool { return strings.EqualFold(filepath.Base(bare), name) }
	for _, pass := range []bool{true, false} {
		for _, bare := range bares {
			if named(bare) == pass && strings.EqualFold(gitctl.RepoSlug(bare), nameWithOwner) {
				return bare
			}
		}
	}
	return ""
}

// parseRepoNumber splits a row's leading "owner/repo#12" field, after the marker.
func parseRepoNumber(line string) (string, int, bool) {
	field := strings.TrimSpace(strings.TrimPrefix(line, repo.AgentMarker))
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
