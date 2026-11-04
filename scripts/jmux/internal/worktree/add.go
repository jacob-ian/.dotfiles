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
	"jmux/internal/prompt"
	"jmux/internal/repo"
	"jmux/internal/session"
	"jmux/internal/spinner"
)

// AddWorktree picks a branch for bareRoot, creates its worktree, and opens a session.
func AddWorktree(bareRoot string) error {
	branches, err := gitctl.RemoteBranches(bareRoot)
	if err != nil {
		return fmt.Errorf("list branches: %s", gitctl.CleanErr(err))
	}

	branchName, err := fzfutil.PickOrQuery(branches, fzfutil.Options{Prompt: "branch> "})
	if err != nil || branchName == "" {
		return nil
	}

	prompts, err := prompt.Load(prompt.Path())
	if err != nil {
		notify.Errorf("prompts.yaml: %v", err)
	}
	promptName := ""
	if len(prompts.Prompts) > 0 {
		promptName = prompt.Pick(prompts)
	}

	worktreePath := filepath.Join(bareRoot, branchName)
	createBranch := !gitctl.RefExists(bareRoot, branchName)
	if err := spinner.Run(fmt.Sprintf("creating %s…", branchName), func(phase chan<- string) error {
		// Best effort: a failed fetch (offline) must not stop the add.
		base := ""
		if def := gitctl.DefaultBranch(bareRoot); def != "" {
			phase <- "fetching " + def + "…"
			_ = gitctl.FetchBranch(bareRoot, def)
			base = "origin/" + def
			if mainWt := filepath.Join(bareRoot, def); repo.IsDir(mainWt) {
				phase <- "updating " + def + "…"
				_ = gitctl.FastForward(mainWt, base)
			}
		}
		// A typed name may be a remote branch not yet fetched: check it out rather
		// than shadowing it with a fresh branch off the default.
		if createBranch {
			phase <- "fetching " + branchName + "…"
			if gitctl.FetchBranch(bareRoot, branchName) == nil {
				createBranch = false
			}
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

	opening := ""
	if promptName != "" {
		opening, err = prompts.Render(promptName, filepath.Base(bareRoot), worktreePath, prompt.SkillsDir())
		if err != nil {
			notify.Errorf("prompt %s: %v", promptName, err)
		}
	}
	return session.Open(worktreePath, session.OpenOptions{
		WithClaude: true,
		InstallCmd: DetectInstallCmd(worktreePath),
		Prompt:     opening,
	})
}

// CopyEnvFiles copies the default checkout's gitignored local config (.env*,
// claude's settings.local.json) into newPath.
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
	local := filepath.Join(".claude", "settings.local.json")
	if _, err := os.Stat(filepath.Join(mainWt, local)); err == nil {
		if err := os.MkdirAll(filepath.Join(newPath, ".claude"), 0o755); err == nil {
			copyFile(filepath.Join(mainWt, local), filepath.Join(newPath, local))
		}
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

// DetectInstallCmd returns dir's dependency-install command from its lockfile, or "".
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
