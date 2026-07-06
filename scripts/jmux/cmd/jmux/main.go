package main

import (
	"fmt"
	"os"
	"strings"

	"jmux/internal/claudectl"
	"jmux/internal/clone"
	"jmux/internal/notify"
	"jmux/internal/pr"
	"jmux/internal/session"
	"jmux/internal/workspace"
	"jmux/internal/worktree"
)

// main reports command failures (the sole notify.Error point) and maps them to
// the exit code. Commands report their own notify.Info-level outcomes.
func main() {
	if err := run(os.Args[1:]); err != nil {
		notify.Error(err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return session.RunPicker()
	}
	switch args[0] {
	case "workspace":
		return runWorkspace(args[1:])
	case "worktree":
		return runWorktree(args[1:])
	case "pr":
		return runPR(args[1:])
	case "repo":
		return runRepo(args[1:])
	case "fzf":
		return runFzf(args[1:])
	case "claude":
		return runClaude(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		return badUsage("jmux: unknown subcommand %q", args[0])
	}
}

func runWorkspace(args []string) error {
	if len(args) == 0 {
		return workspace.RunPicker()
	}
	switch args[0] {
	case "add":
		return workspace.RunAdd()
	case "remove":
		return workspace.RunRemove(args[1:])
	default:
		return badUsage("jmux workspace: unknown subcommand %q", args[0])
	}
}

func runWorktree(args []string) error {
	if len(args) == 0 {
		return worktree.RunPicker()
	}
	switch args[0] {
	case "add":
		return worktree.RunAdd()
	case "remove":
		return worktree.RunRemove(args[1:])
	default:
		return badUsage("jmux worktree: unknown subcommand %q", args[0])
	}
}

func runRepo(args []string) error {
	if len(args) == 0 {
		return badUsage("jmux repo: expected clone <url>")
	}
	switch args[0] {
	case "clone":
		if len(args) != 2 {
			return badUsage("jmux repo clone: expected <url>")
		}
		return clone.Run(args[1])
	default:
		return badUsage("jmux repo: unknown subcommand %q", args[0])
	}
}

func runPR(args []string) error {
	if len(args) == 0 {
		return pr.RunAssigned()
	}
	if num, ok := pr.ParseNumber(args[0]); ok {
		return pr.RunNumber(num)
	}
	return pr.RunRepo(args[0])
}

// runClaude handles `jmux claude`: notify/focus are jmux's own subcommands;
// anything else passes through as arguments to the claude binary.
func runClaude(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "notify":
			return claudectl.RunNotify()
		case "focus":
			return claudectl.RunFocus(args[1:])
		}
	}
	if err := claudectl.Run(args); err != nil {
		return fmt.Errorf("claude: %w", err)
	}
	return nil
}

// runFzf dispatches `jmux fzf <picker> items|preview` — plumbing invoked by
// fzf bindings, not humans. Void by design: stdout belongs to fzf, so these
// report locally rather than propagate.
func runFzf(args []string) error {
	if len(args) >= 2 {
		switch args[0] + " " + args[1] {
		case "workspace items":
			workspace.RunItems()
			return nil
		case "workspace preview":
			workspace.RunPreview(args[2:])
			return nil
		case "worktree items":
			worktree.RunItems()
			return nil
		case "pr items":
			pr.RunItems(args[2:])
			return nil
		case "pr preview":
			pr.RunPreview(args[2:])
			return nil
		}
	}
	return badUsage("jmux fzf: unknown command %q", strings.Join(args, " "))
}

// badUsage prints the complaint and usage to stderr and exits 1 directly:
// argument mistakes are made at a terminal, where stderr beats a status-line
// notification.
func badUsage(format string, args ...any) error {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	usage()
	os.Exit(1)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  jmux                          Open the all-dirs picker
  jmux workspace                Open the workspace overview: open sessions +
                                feature worktrees (ctrl-t adds, ctrl-x removes)
  jmux workspace add            Pick a bare repo, then create a worktree from it
  jmux workspace remove --path P [--quiet]
                                Remove a worktree-backed workspace, or close the
                                session for a plain one (never deletes the dir)
  jmux worktree                 Open the worktrees picker (ctrl-x removes)
  jmux worktree add             Create a worktree from a remote branch
  jmux worktree remove [--path P] [--quiet]
                                Remove a worktree (interactive without --path)
  jmux pr                       Review queue: PRs across all your repos that await
                                your review, are assigned to you, or you created
                                (body + threads in the preview; ctrl-r refreshes),
                                then check one out into a worktree and open it
  jmux pr <dir>                 Open PRs for the repo at <dir>, regardless of
                                assignment (jmux pr . for the current repo)
  jmux pr <num>                 Review PR <num> in the current repo directly
  jmux repo clone <url>         Clone as a bare repo into a scan root, set up
                                origin refs, and open the default-branch worktree
  jmux claude [args...]         Launch claude paired with the nvim instance
                                whose workspace contains the current directory
  jmux claude notify            Notification hook: post a macOS alert that
                                focuses this pane on click (via jmux claude focus)
  jmux fzf <picker> items|preview
                                Internal: list rows / render previews for the
                                pickers' fzf bindings`)
}
