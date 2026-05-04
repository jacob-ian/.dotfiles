package worktree

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"jmux/internal/fzfutil"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/tmuxctl"
)

// RunAdd handles `jmux worktree add`.
func RunAdd(args []string) {
	cwd, err := os.Getwd()
	if err != nil {
		tmuxctl.DisplayMessage("Failed to read cwd")
		return
	}
	bareRoot := repo.GitCommonDir(cwd)
	if bareRoot == "" {
		tmuxctl.DisplayMessage("Not in a bare repo worktree")
		return
	}

	branches, err := remoteBranches(bareRoot)
	if err != nil {
		tmuxctl.DisplayMessage("Failed to list branches")
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
	if !runGit(bareRoot, "worktree", "add", worktreePath, branchName) &&
		!runGit(bareRoot, "worktree", "add", worktreePath, "-b", branchName) {
		tmuxctl.DisplayMessage(fmt.Sprintf("Failed to create worktree '%s'", branchName))
		return
	}

	copyEnvFiles(bareRoot, worktreePath)

	session.Open(worktreePath, session.OpenOptions{WithClaude: true})

	if installCmd := detectInstallCmd(worktreePath); installCmd != "" {
		sessionName := session.Name(worktreePath)
		shellCmd := fmt.Sprintf("%s || { echo; echo '[install failed — press enter to close]'; read; }", installCmd)
		tmuxctl.NewWindow(sessionName, "install", worktreePath, shellCmd, true)
	}
}

func remoteBranches(bareRoot string) ([]string, error) {
	cmd := exec.Command("git", "-C", bareRoot, "branch", "-r")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var branches []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.Contains(line, "HEAD") {
			continue
		}
		line = strings.TrimPrefix(line, "origin/")
		branches = append(branches, line)
	}
	return branches, nil
}

func runGit(dir string, args ...string) bool {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	return cmd.Run() == nil
}

// copyEnvFiles copies any .env* file from the default-branch worktree into newPath.
func copyEnvFiles(bareRoot, newPath string) {
	defaultBranch := defaultBranchName(bareRoot)
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

func defaultBranchName(bareRoot string) string {
	cmd := exec.Command("git", "-C", bareRoot, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/")
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
