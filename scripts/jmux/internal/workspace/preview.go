package workspace

import (
	"flag"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"jmux/internal/gitctl"
	"jmux/internal/nvimctl"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/tmuxctl"
	"jmux/internal/worktree"
)

// RunPreview prints a one-screen summary of the workspace at --path. fzf runs
// it on every cursor move, so git calls are fanned out in parallel.
func RunPreview(args []string) {
	fs := flag.NewFlagSet("workspace preview", flag.ContinueOnError)
	path := fs.String("path", "", "Workspace path to summarise")
	if err := fs.Parse(args); err != nil || *path == "" {
		return
	}

	bareRoot := repo.FindBareRoot(*path)

	live := make(chan string, 1)
	go func() { live <- livePane(session.Name(*path)) }()

	var (
		branch        string
		status        string
		defaultBranch string
		removable     bool
	)
	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); branch = gitctl.CurrentBranch(*path) }()
	go func() { defer wg.Done(); status = gitctl.ShortStatus(*path) }()
	go func() {
		defer wg.Done()
		if bareRoot != "" {
			defaultBranch = gitctl.DefaultBranch(bareRoot)
		}
	}()
	go func() { defer wg.Done(); removable = worktree.IsManagedWorktree(*path) }()
	wg.Wait()

	if branch == "" {
		if bareRoot == "" {
			branch = filepath.Base(*path)
		} else {
			branch = "(detached)"
		}
	}
	repoName := ""
	if bareRoot != "" {
		repoName = filepath.Base(bareRoot)
	}
	if repoName != "" {
		fmt.Printf("%s · %s\n", repoName, branch)
	} else {
		fmt.Println(branch)
	}

	dirty := status != ""
	verdict := ""
	logOutput := ""

	if defaultBranch != "" {
		// The failed comparison doubles as the ref-existence check, saving a
		// rev-parse per preview; scratch has no remote so falls to the local default.
		ref := "origin/" + defaultBranch
		var ahead, behind int
		var ok bool
		ahead, behind, logOutput, ok = compare(*path, ref)
		if !ok {
			ref = defaultBranch
			ahead, behind, logOutput, ok = compare(*path, ref)
		}

		if !ok {
			verdict = "no upstream comparison"
		} else {
			var parts []string
			if ahead > 0 || behind > 0 {
				parts = append(parts, fmt.Sprintf("↑%d ↓%d vs %s", ahead, behind, ref))
			}
			if dirty {
				parts = append(parts, "uncommitted changes")
			}
			switch {
			case len(parts) > 0:
				verdict = strings.Join(parts, " · ")
			case removable:
				verdict = "no unique commits — safe to remove"
			default:
				verdict = "up to date with " + ref
			}
		}
	} else if dirty {
		verdict = "uncommitted changes"
	}

	if verdict != "" {
		fmt.Println(verdict)
	}
	fmt.Println()

	if pane := <-live; pane != "" {
		fmt.Println(pane)
		return
	}

	if dirty {
		fmt.Println("uncommitted changes:")
		fmt.Println(status)
		fmt.Println()
	}

	if logOutput != "" {
		fmt.Println("commits from HEAD:")
		fmt.Println(logOutput)
	}
}

func compare(path, ref string) (ahead, behind int, log string, ok bool) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); ahead, behind, ok = gitctl.AheadBehind(path, ref) }()
	go func() { defer wg.Done(); log = gitctl.LogOneline(path, ref+"..HEAD", 10) }()
	wg.Wait()
	return ahead, behind, log, ok
}

// livePane captures the session's claude window, else its nvim window (an
// alt-screen TUI, so only the visible pane); "" when there is no session.
func livePane(sessionName string) string {
	windows := tmuxctl.WindowNames(sessionName)
	if slices.Contains(windows, "claude") {
		if capture := tmuxctl.CapturePane(sessionName+":claude", 40); capture != "" {
			return "claude:\n" + capture
		}
	}
	if slices.Contains(windows, nvimctl.WindowName) {
		if capture := tmuxctl.CapturePane(sessionName+":"+nvimctl.WindowName, 0); capture != "" {
			return "nvim:\n" + capture
		}
	}
	return ""
}
