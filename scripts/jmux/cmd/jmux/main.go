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

func main() {
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
	if args[0] == "sync" {
		return pr.RunSync()
	}
	if num, ok := pr.ParseNumber(args[0]); ok {
		return pr.RunNumber(cfg, num)
	}
	return pr.RunRepo(cfg, args[0])
}

// runClaude handles `jmux claude`; anything but hook/focus passes through to claude.
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

// runFzf dispatches the plumbing fzf bindings call. stdout belongs to fzf, so
// these report locally rather than return errors.
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

// badUsage exits via stderr rather than the status line: argument mistakes
// happen at a terminal.
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
  jmux pr sync                  Bring this worktree up to its branch on origin
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
