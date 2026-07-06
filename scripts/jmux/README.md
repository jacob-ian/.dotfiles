# jmux

A tmux workspace CLI built around one model: repos are cloned bare under a
scan root, branches are checked out as worktrees inside the bare root, and
every worktree maps to a tmux session. Everything jmux does — the pickers,
PR review, claude pairing, notifications — creates, manages, opens, or feeds
those workspaces.

Run `jmux help` for the command surface, or see the keybindings in
[`tmux.conf`](../../tmux/tmux.conf).

## Does a feature belong?

The test for adding a command: **does it create, manage, open, or feed a
workspace?** If it doesn't touch that lifecycle, it goes somewhere else.
`repo clone` (creates the layout), `pr` (feeds branches into it), and
`claude hook` (serves the sessions it opens) all pass; a linter or deploy
helper would not.

## Conventions

- **Namespace by consumer.** Human-invoked commands are top-level groups
  (`workspace`, `worktree`, `pr`, `repo`, `claude`); commands invoked by fzf
  bindings live under `jmux fzf <picker> items|preview`. Flags parameterize a
  command, never switch who its output is for.
- **Errors propagate to `main`**, the sole `notify.Error` point and owner of
  the exit code. `notify.Info` (successes, "nothing to do") stays in the
  `Run*` command functions. Cancelled pickers return nil, silently.
- **fzf plumbing is void.** Its stdout belongs to fzf and its exit codes are
  ignored, so it reports locally and never propagates.
- **Interactive/operation split.** `Run*` functions own pickers and sessions;
  the operation underneath (`Remove`, `AddWorktree`, `clone`) is a plain
  function returning errors — that's also the test seam.

## Build

```bash
make build   # -> bin/jmux
make test
```
