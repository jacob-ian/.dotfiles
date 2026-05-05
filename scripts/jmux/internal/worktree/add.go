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
)

// RunAdd handles `jmux worktree add`.
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

	branches, err := gitctl.RemoteBranches(bareRoot)
	if err != nil {
		notify.Error("Failed to list branches")
		return
	}

	branchName, err := fzfutil.Run(branches, fzfutil.Options{
		Prompt:     "branch> ",
		PrintQuery: true,
	})
	if err != nil || branchName == "" {
		return
	}

	worktreePath := filepath.Join(bareRoot, branchName)

	// Try existing branch first, then create new branch.
	if err := gitctl.WorktreeAdd(bareRoot, worktreePath, branchName, false); err != nil {
		if err := gitctl.WorktreeAdd(bareRoot, worktreePath, branchName, true); err != nil {
			notify.Error(fmt.Sprintf("Failed to create worktree '%s'", branchName))
			return
		}
	}

	copyEnvFiles(bareRoot, worktreePath)

	if err := session.Open(worktreePath, session.OpenOptions{
		WithClaude: true,
		InstallCmd: detectInstallCmd(worktreePath),
	}); err != nil {
		notify.Error(err.Error())
	}
}

// copyEnvFiles copies any .env* file from the default-branch worktree into newPath.
func copyEnvFiles(bareRoot, newPath string) {
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

func detectInstallCmd(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, "yarn.lock")); err == nil {
		return "yarn install"
	}
	if _, err := os.Stat(filepath.Join(dir, "package-lock.json")); err == nil {
		return "npm install"
	}
	return ""
}
