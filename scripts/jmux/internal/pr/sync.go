package pr

import (
	"errors"
	"fmt"
	"os"

	"jmux/internal/gitctl"
)

// Outcome is what Sync did; the zero value means no sync ran.
type Outcome string

const (
	Current       Outcome = "current"
	FastForwarded Outcome = "fast-forwarded"
	Reset         Outcome = "reset"
	Diverged      Outcome = "diverged"
)

type SyncResult struct {
	Outcome       Outcome
	Branch        string
	Reason        string // why a Diverged worktree was left alone
	Ahead, Behind int
}

const (
	reasonDirty   = "uncommitted changes"
	reasonCommits = "local commits"
)

func (r SyncResult) String() string {
	switch r.Outcome {
	case Current:
		return r.Branch + " is up to date"
	case FastForwarded:
		return "fast-forwarded " + r.Branch
	case Reset:
		return "reset " + r.Branch + " to origin (its history was rewritten)"
	case Diverged:
		return fmt.Sprintf("%s has %s; left as is (%d ahead, %d behind origin)", r.Branch, r.Reason, r.Ahead, r.Behind)
	}
	return ""
}

// Sync fast-forwards the worktree to origin, or after a force-push resets it
// when it holds nothing of its own (HEAD was origin's old head, tree clean).
func Sync(bareRoot, worktree, branch string) (SyncResult, error) {
	res := SyncResult{Branch: branch}
	remote := "origin/" + branch

	head, ok := gitctl.RevParse(worktree, "HEAD")
	if !ok {
		return res, errors.New("no commit checked out")
	}
	before, hadRemote := gitctl.RevParse(worktree, remote)
	clean := gitctl.IsClean(worktree)

	if err := gitctl.FetchBranch(bareRoot, branch); err != nil {
		return res, err
	}
	after, ok := gitctl.RevParse(worktree, remote)
	if !ok {
		return res, errors.New(remote + " not found after fetch")
	}

	switch {
	case head == after:
		res.Outcome = Current
	case gitctl.FastForward(worktree, remote) == nil:
		res.Outcome = FastForwarded
	case hadRemote && head == before && clean:
		if err := gitctl.ResetHard(worktree, remote); err != nil {
			return res, err
		}
		res.Outcome = Reset
	default:
		res.Outcome = Diverged
		res.Reason = reasonCommits
		if hadRemote && head == before {
			res.Reason = reasonDirty
		}
		res.Ahead, res.Behind, _ = gitctl.AheadBehind(worktree, remote)
	}
	return res, nil
}

// RunSync handles `jmux pr sync`; the outcome goes to stdout for nvim.
func RunSync() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("reading cwd: %w", err)
	}
	branch := gitctl.CurrentBranch(cwd)
	if branch == "" {
		return errors.New("no branch checked out")
	}
	// A plain clone has no common dir; it can fetch from itself.
	bareRoot := gitctl.CommonDir(cwd)
	if bareRoot == "" {
		bareRoot = cwd
	}
	res, err := Sync(bareRoot, cwd, branch)
	if err != nil {
		return fmt.Errorf("sync %s: %s", branch, gitctl.CleanErr(err))
	}
	fmt.Println(res)
	return nil
}
