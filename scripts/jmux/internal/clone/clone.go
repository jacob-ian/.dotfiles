// Package clone implements `jmux repo clone`: clone a repo bare under a scan
// root and establish the layout the rest of jmux assumes — fetch refspec,
// origin/HEAD, and a worktree for the default branch.
package clone

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"jmux/internal/fzfutil"
	"jmux/internal/gitctl"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/spinner"
	"jmux/internal/worktree"
)

// Run handles `jmux repo clone <url>`: pick a scan root, clone into it, and
// open a session on the default-branch worktree.
func Run(url string) error {
	name := repoName(url)
	if name == "" {
		return fmt.Errorf("cannot derive a repo name from %q", url)
	}

	var roots []string
	for _, r := range repo.ExpandPaths(repo.ScanRoots) {
		if repo.IsDir(r) {
			roots = append(roots, r)
		}
	}
	if len(roots) == 0 {
		return errors.New("no scan roots exist")
	}
	root, err := fzfutil.Pick(roots, fzfutil.Options{Prompt: "clone into> "})
	if err != nil || root == "" {
		return nil
	}

	dest := filepath.Join(repo.TrimSlash(root), name)
	wtPath, err := clone(url, dest)
	if err != nil {
		return err
	}
	return session.Open(wtPath, session.OpenOptions{
		InstallCmd: worktree.DetectInstallCmd(wtPath),
	})
}

// clone bare-clones url to dest and returns the default-branch worktree path.
func clone(url, dest string) (string, error) {
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("%s already exists", dest)
	}

	var wtPath string
	err := spinner.Run("cloning "+filepath.Base(dest)+"…", func(phase chan<- string) error {
		if err := gitctl.CloneBare(filepath.Dir(dest), url, dest); err != nil {
			return fmt.Errorf("git clone --bare: %s", gitctl.CleanErr(err))
		}
		phase <- "setting up origin refs…"
		if err := gitctl.SetupBareRemote(dest); err != nil {
			return fmt.Errorf("setting up origin refs: %s", gitctl.CleanErr(err))
		}
		def := gitctl.DefaultBranch(dest)
		if def == "" {
			return errors.New("could not detect the default branch")
		}
		phase <- "creating " + def + " worktree…"
		wtPath = filepath.Join(dest, def)
		if err := gitctl.WorktreeAdd(dest, wtPath, def, "", false); err != nil {
			return fmt.Errorf("git worktree add: %s", gitctl.CleanErr(err))
		}
		return nil
	})
	return wtPath, err
}

// repoName derives the repo directory name from a clone URL (scp, https, or
// ssh:// forms).
func repoName(url string) string {
	s := strings.TrimSuffix(strings.TrimRight(url, "/"), ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	return s
}
