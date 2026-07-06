package worktree

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"jmux/internal/fzfutil"
	"jmux/internal/gitctl"
	"jmux/internal/notify"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/spinner"
)

// RunAdd handles `jmux worktree add`: resolve the bare repo from cwd, then run
// the branch flow against it.
func RunAdd(args []string) {
	cwd, err := os.Getwd()
	if err != nil {
		notify.Error("Failed to read cwd")
		return
	}
	bareRoot := gitctl.CommonDir(cwd)
	if bareRoot == "" {
		notify.Error("Not in a bare repo worktree")
		return
	}
	if err := AddWorktree(bareRoot); err != nil {
		notify.Error(err.Error())
	}
}

// AddWorktree runs the branch picker for bareRoot, creates the worktree, copies
// env files from the default branch, and opens a session with claude + install
// windows. Returns nil when the picker is cancelled.
func AddWorktree(bareRoot string) error {
	branches, err := gitctl.RemoteBranches(bareRoot)
	if err != nil {
		return fmt.Errorf("list branches: %s", gitctl.CleanErr(err))
	}

	branchName, err := fzfutil.PickOrQuery(branches, fzfutil.Options{Prompt: "branch> "})
	if err != nil || branchName == "" {
		return nil
	}

	worktreePath := filepath.Join(bareRoot, branchName)
	createBranch := !gitctl.RefExists(bareRoot, branchName)
	if err := spinner.Run(fmt.Sprintf("creating %s…", branchName), func(phase chan<- string) error {
		// Refresh the default branch so a new branch starts from the latest (not a
		// stale local HEAD) and a later PR diff resolves against an up-to-date base.
		// Best effort: a failed fetch (e.g. offline) shouldn't stop the add.
		base := ""
		if def := gitctl.DefaultBranch(bareRoot); def != "" {
			phase <- "fetching " + def + "…"
			_ = gitctl.FetchBranch(bareRoot, def)
			base = "origin/" + def
		}
		if err := gitctl.WorktreeAdd(bareRoot, worktreePath, branchName, base, createBranch); err != nil {
			flag := ""
			if createBranch {
				flag = " -b " + branchName
			}
			return fmt.Errorf("git worktree add%s: %s", flag, gitctl.CleanErr(err))
		}
		phase <- "copying env files…"
		CopyEnvFiles(bareRoot, worktreePath)
		return nil
	}); err != nil {
		return err
	}

	return session.Open(worktreePath, session.OpenOptions{
		WithClaude: true,
		InstallCmd: DetectInstallCmd(worktreePath),
	})
}

// CopyEnvFiles copies any .env* file from the default-branch worktree into newPath.
func CopyEnvFiles(bareRoot, newPath string) {
	defaultBranch := gitctl.DefaultBranch(bareRoot)
	if defaultBranch == "" {
		return
	}
	mainWt := filepath.Join(bareRoot, defaultBranch)
	if !repo.IsDir(mainWt) || mainWt == newPath {
		return
	}
	entries, err := os.ReadDir(mainWt)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), ".env") {
			continue
		}
		src := filepath.Join(mainWt, e.Name())
		dst := filepath.Join(newPath, e.Name())
		copyFile(src, dst)
	}
}

func copyFile(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return
	}
	defer out.Close()
	io.Copy(out, in)
}

// DetectInstallCmd returns the dependency-install command for dir based on its
// lockfile, or "" when dir needs no install step.
func DetectInstallCmd(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, "yarn.lock")); err == nil {
		return "yarn install"
	}
	if _, err := os.Stat(filepath.Join(dir, "package-lock.json")); err == nil {
		return "npm install"
	}
	if _, err := os.Stat(filepath.Join(dir, "pnpm-lock.yaml")); err == nil {
		return "pnpm install"
	}
	return ""
}
