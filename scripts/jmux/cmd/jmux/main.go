package main

import (
	"fmt"
	"os"
	"strings"

	"jmux/internal/claudectl"
	"jmux/internal/clone"
	"jmux/internal/notify"
	"jmux/internal/pr"
	"jmux/internal/repo"
	"jmux/internal/scratch"
	"jmux/internal/session"
	"jmux/internal/statusbox"
	"jmux/internal/workspace"
)

// main reports command failures (the sole notify.Error point) and maps them to
// the exit code. Commands report their own notify.Info-level outcomes.
func main() {
	// Tag renderers and notice sources wire here, not in init, so the
	// binary's badge and notification vocabulary is explicit.
	claudectl.Register()
	pr.Register()

	if err := run(os.Args[1:]); err != nil {
		notify.Error(err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg := repo.Default()
	if len(args) == 0 {
		return session.RunPicker(cfg)
	}
	switch args[0] {
	case "workspace":
		return runWorkspace(cfg, args[1:])
	case "pr":
		return runPR(cfg, args[1:])
	case "repo":
		return runRepo(cfg, args[1:])
	case "scratch":
		return scratch.Run(cfg, args[1:])
	case "fzf":
		return runFzf(cfg, args[1:])
	case "claude":
		return runClaude(args[1:])
	case "statusline":
		return runStatusline(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		return badUsage("jmux: unknown subcommand %q", args[0])
	}
}

func runWorkspace(cfg repo.Config, args []string) error {
	if len(args) == 0 {
		return workspace.RunPicker(cfg)
	}
	switch args[0] {
	case "add":
		return workspace.RunAdd(cfg)
	case "remove":
		return workspace.RunRemove(cfg, args[1:])
	default:
		return badUsage("jmux workspace: unknown subcommand %q", args[0])
	}
}

func runRepo(cfg repo.Config, args []string) error {
	if len(args) == 0 {
		return badUsage("jmux repo: expected clone <url>")
	}
	switch args[0] {
	case "clone":
		if len(args) != 2 {
			return badUsage("jmux repo clone: expected <url>")
		}
		return clone.Run(cfg, args[1])
	default:
		return badUsage("jmux repo: unknown subcommand %q", args[0])
	}
}

func runPR(cfg repo.Config, args []string) error {
	if len(args) == 0 {
		return pr.RunAssigned(cfg)
	}
	if num, ok := pr.ParseNumber(args[0]); ok {
		return pr.RunNumber(cfg, num)
	}
	return pr.RunRepo(cfg, args[0])
}

// runClaude handles `jmux claude`: hook/focus are jmux's own subcommands;
// anything else passes through as arguments to the claude binary.
func runClaude(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "hook":
			return claudectl.RunHook()
		case "focus":
			return claudectl.RunFocus(args[1:])
		}
	}
	if err := claudectl.Run(args); err != nil {
		return fmt.Errorf("claude: %w", err)
	}
	return nil
}

// runStatusline handles `jmux statusline`: render republishes the multibox;
// click is the status-line mouse binding's dispatch, not for humans.
func runStatusline(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "render":
			statusbox.Publish()
			return nil
		case "click":
			return statusbox.RunClick(args[1:])
		}
	}
	return badUsage("jmux statusline: expected render or click <range>")
}

// runFzf dispatches `jmux fzf <picker> items|preview` — plumbing invoked by
// fzf bindings, not humans. Void by design: stdout belongs to fzf, so these
// report locally rather than propagate.
func runFzf(cfg repo.Config, args []string) error {
	if len(args) >= 2 {
		switch args[0] + " " + args[1] {
		case "workspace items":
			workspace.RunItems(cfg)
			return nil
		case "workspace preview":
			workspace.RunPreview(args[2:])
			return nil
		case "scratch items":
			scratch.RunItems(cfg)
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
  jmux workspace                Overview picker: open sessions + feature worktrees
  jmux workspace add            Pick a bare repo and create a worktree in it
  jmux workspace remove --path P [--quiet]
                                Remove the workspace (never deletes a plain dir)
  jmux pr                       Review queue: check a PR out into a worktree
  jmux pr <dir>                 PRs for the repo at <dir> (. for current)
  jmux pr <num>                 Review PR <num> in the current repo
  jmux repo clone <url>         Bare-clone into a scan root and open main
  jmux scratch                  Scratch sessions picker (ctrl-t new, ctrl-x harvest)
  jmux scratch new [name]       Create a dated scratch session
  jmux claude [args...]         Launch claude paired with this workspace's nvim
  jmux claude hook              Hook entry point: status badges + notifications
  jmux statusline render        Republish the status-line multibox
  jmux statusline click <range> Internal status-line mouse plumbing
  jmux fzf <picker> items|preview
                                Internal fzf plumbing`)
}
