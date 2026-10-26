// Package scratch implements `jmux scratch`: sessions are worktrees named
// YYYY-MM-DD[-name] under Config.ScratchRoot, harvested into the default branch
// by worktree.Remove.
package scratch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"jmux/internal/fzfutil"
	"jmux/internal/gitctl"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/spinner"
	"jmux/internal/worktree"
)

var datePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(-|$)`)

// Run handles `jmux scratch` (the picker) and `jmux scratch new [name]`.
func Run(cfg repo.Config, args []string) error {
	bare, err := bareRoot(cfg)
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "new" {
		query := strings.Join(args[1:], " ")
		if query == "" {
			// No items: the prefilled date would fuzzy-match today's sessions
			// and enter would open one instead of creating.
			query, err = fzfutil.PickOrQuery(nil, fzfutil.Options{
				Prompt: "new scratch> ",
				Query:  time.Now().Format(time.DateOnly) + "-",
			})
			if err != nil || query == "" {
				return nil
			}
		}
		return create(cfg, query)
	}
	self := fzfutil.Self()
	sel, err := fzfutil.Pick(rows(bare), fzfutil.Options{
		Prompt: "scratch> ",
		Header: "ctrl-t: new session · ctrl-x: remove · ctrl-/: toggle preview",
		Bindings: []string{
			fmt.Sprintf("ctrl-t:become(%s scratch new)", self),
			fmt.Sprintf("ctrl-x:execute-silent(%s worktree remove --path {2} --quiet)+reload(%s fzf scratch items)", self, self),
			"ctrl-/:toggle-preview",
		},
		Preview:       fmt.Sprintf("%s fzf workspace preview --path {2}", self),
		PreviewWindow: "follow",
		Delimiter:     "\t",
		WithNth:       "1",
	})
	if err != nil || sel == "" {
		return nil
	}
	path := fzfutil.RowPath(sel)
	return session.Open(path, session.OpenOptions{WithClaude: true, Dir: cfg.WorkDir(path)})
}

// RunItems handles `jmux fzf scratch items`: the picker's reload rows.
func RunItems(cfg repo.Config) {
	bare, err := bareRoot(cfg)
	if err != nil {
		return
	}
	fmt.Println(strings.Join(rows(bare), "\n"))
}

func bareRoot(cfg repo.Config) (string, error) {
	if repo.FindBareRoot(cfg.ScratchRoot) != cfg.ScratchRoot {
		return "", fmt.Errorf("no scratch repo at %s (bare-clone one there first)", cfg.ScratchRoot)
	}
	return cfg.ScratchRoot, nil
}

// rows are newest first so today's work sits at the top.
func rows(bare string) []string {
	wts := repo.FeatureWorktrees(bare)
	slices.Sort(wts)
	slices.Reverse(wts)
	out := make([]string, len(wts))
	for i, w := range wts {
		out[i] = fzfutil.Row(filepath.Base(w), w)
	}
	return out
}

func create(cfg repo.Config, query string) error {
	bare := cfg.ScratchRoot
	name := SessionName(query, time.Now().Format(time.DateOnly), func(n string) bool {
		return repo.IsDir(filepath.Join(bare, n)) || gitctl.RefExists(bare, n)
	})
	path := filepath.Join(bare, name)
	if err := spinner.Run(fmt.Sprintf("creating %s…", name), func(chan<- string) error {
		def := gitctl.DefaultBranch(bare)
		if def == "" {
			return errors.New("scratch repo has no default branch")
		}
		if err := gitctl.WorktreeAdd(bare, path, name, def, true); err != nil {
			return fmt.Errorf("git worktree add: %s", gitctl.CleanErr(err))
		}
		// The session works inside <name>/ (see Config.WorkDir) so its files are
		// already where history keeps them; harvest has nothing to move.
		if err := os.MkdirAll(filepath.Join(path, name), 0o755); err != nil {
			return err
		}
		worktree.CopyEnvFiles(bare, path)
		return nil
	}); err != nil {
		return err
	}
	return session.Open(path, session.OpenOptions{WithClaude: true, Dir: cfg.WorkDir(path)})
}

// SessionName: empty → today; a query with its own date → as is; otherwise
// today-<slug>. Taken names get -2, -3… appended.
func SessionName(query, today string, taken func(string) bool) string {
	slug := strings.Trim(strings.Join(strings.Fields(strings.ToLower(query)), "-"), "-")
	name := today
	switch {
	case slug == "":
	case datePrefix.MatchString(slug):
		name = slug
	default:
		name = today + "-" + slug
	}
	candidate := name
	for i := 2; taken(candidate); i++ {
		candidate = fmt.Sprintf("%s-%d", name, i)
	}
	return candidate
}
