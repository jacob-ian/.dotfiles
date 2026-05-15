package main

import (
	"fmt"
	"os"

	"jmux/internal/claudectl"
	"jmux/internal/session"
	"jmux/internal/worktree"
)

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		session.RunPicker()
		return
	}

	switch args[0] {
	case "worktree":
		runWorktree(args[1:])
	case "claude":
		if err := claudectl.Run(args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "jmux claude: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "jmux: unknown subcommand %q\n", args[0])
		usage()
		os.Exit(1)
	}
}

func runWorktree(args []string) {
	if len(args) == 0 {
		worktree.RunPicker(nil)
		return
	}
	switch args[0] {
	case "add":
		worktree.RunAdd(args[1:])
	case "remove":
		worktree.RunRemove(args[1:])
	case "preview":
		worktree.RunPreview(args[1:])
	case "--print":
		worktree.RunPicker(args)
	default:
		fmt.Fprintf(os.Stderr, "jmux worktree: unknown subcommand %q\n", args[0])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  jmux                          Open the all-dirs picker
  jmux worktree                 Open the worktrees picker (ctrl-x removes)
  jmux worktree add             Create a worktree from a remote branch
  jmux worktree remove          Remove a worktree (interactive)
  jmux worktree remove --path P --quiet
                                Remove a specific worktree (used by ctrl-x bind)
  jmux worktree preview --path P
                                Print a summary of P (used by fzf --preview)
  jmux claude [args...]         Launch claude paired with the nvim instance
                                whose workspace contains the current directory`)
}
