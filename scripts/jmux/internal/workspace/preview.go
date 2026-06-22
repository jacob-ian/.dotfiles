package workspace

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"jmux/internal/gitctl"
	"jmux/internal/nvimctl"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/tmuxctl"
	"jmux/internal/worktree"
)

// RunPreview prints a one-screen summary of the workspace at --path. Invoked by
// fzf on every cursor move, so it must be fast and never error to stderr.
//
// Git calls are fanned out in two waves of goroutines to overlap their
// subprocess + IO costs. On a typical repo the wall-clock is bounded by the
// slowest single call (`git status`) rather than their sum.
func RunPreview(args []string) {
	fs := flag.NewFlagSet("workspace preview", flag.ContinueOnError)
	path := fs.String("path", "", "Workspace path to summarise")
	if err := fs.Parse(args); err != nil || *path == "" {
		return
	}

	bareRoot := repo.FindBareRoot(*path)

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

	// For an open session, show a live pane instead of git detail: claude if
	// it's running, otherwise the nvim editor view. nvim is an alt-screen TUI,
	// so capture only the visible pane (no scrollback).
	sessionName := session.Name(*path)
	if tmuxctl.HasSession(sessionName) {
		if tmuxctl.HasWindow(sessionName, "claude") {
			if capture := tmuxctl.CapturePane(sessionName+":claude", 40); capture != "" {
				fmt.Println("claude:")
				fmt.Println(capture)
				return
			}
		}
		if tmuxctl.HasWindow(sessionName, nvimctl.WindowName) {
			if capture := tmuxctl.CapturePane(sessionName+":"+nvimctl.WindowName, 0); capture != "" {
				fmt.Println("nvim:")
				fmt.Println(capture)
				return
			}
		}
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
