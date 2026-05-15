package worktree

import (
	"flag"
	"fmt"
	"path/filepath"
	"sync"

	"jmux/internal/gitctl"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/tmuxctl"
)

// RunPreview prints a one-screen summary of the worktree at --path. Invoked by
// fzf on every cursor move, so it must be fast and never error to stderr.
//
// Git calls are fanned out in two waves of goroutines to overlap their
// subprocess + IO costs. On a typical repo the wall-clock is bounded by the
// slowest single call (`git status`) rather than their sum.
func RunPreview(args []string) {
	fs := flag.NewFlagSet("worktree preview", flag.ContinueOnError)
	path := fs.String("path", "", "Worktree path to summarise")
	if err := fs.Parse(args); err != nil || *path == "" {
		return
	}

	bareRoot := repo.FindBareRoot(*path)

	var (
		branch        string
		status        string
		defaultBranch string
	)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); branch = gitctl.CurrentBranch(*path) }()
	go func() { defer wg.Done(); status = gitctl.ShortStatus(*path) }()
	go func() {
		defer wg.Done()
		if bareRoot != "" {
			defaultBranch = gitctl.DefaultBranch(bareRoot)
		}
	}()
	wg.Wait()

	if branch == "" {
		branch = "(detached)"
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
		ref := "origin/" + defaultBranch
		var (
			ahead, behind int
			ok            bool
		)
		var wg2 sync.WaitGroup
		wg2.Add(2)
		go func() { defer wg2.Done(); ahead, behind, ok = gitctl.AheadBehind(*path, ref) }()
		go func() { defer wg2.Done(); logOutput = gitctl.LogOneline(*path, ref+"..HEAD", 10) }()
		wg2.Wait()

		switch {
		case !ok:
			verdict = "no upstream comparison"
			logOutput = ""
		case ahead == 0 && !dirty:
			verdict = "MERGED — safe to remove"
			logOutput = ""
		case ahead == 0 && dirty:
			verdict = "DIRTY (no unique commits)"
			logOutput = ""
		default:
			verdict = fmt.Sprintf("↑%d ↓%d vs %s", ahead, behind, ref)
			if dirty {
				verdict += " · DIRTY"
			}
		}
	} else if dirty {
		verdict = "DIRTY"
	}

	if verdict != "" {
		fmt.Println(verdict)
	}
	fmt.Println()

	sessionName := session.Name(*path)
	if tmuxctl.HasSession(sessionName) && tmuxctl.HasWindow(sessionName, "claude") {
		capture := tmuxctl.CapturePane(sessionName+":claude", 40)
		if capture != "" {
			fmt.Println("claude:")
			fmt.Println(capture)
			return
		}
	}

	if dirty {
		fmt.Println("Working tree:")
		fmt.Println(status)
		fmt.Println()
	}

	if logOutput != "" {
		fmt.Printf("Unique commits (origin/%s..HEAD):\n", defaultBranch)
		fmt.Println(logOutput)
	}
}
