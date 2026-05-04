package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// ScanRoots are the parent directories under which repos and worktrees live.
var ScanRoots = []string{"$HOME/dev", "$HOME/euc", "$HOME/net"}

// AdditionalDirs are non-repo directories included in the default picker.
var AdditionalDirs = []string{"$HOME/.config", "$HOME/.claude"}

func IsBareRepo(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "refs")); err != nil {
		return false
	}
	return true
}

func FindBareRoot(dir string) string {
	cur := dir
	for {
		if IsBareRepo(cur) {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

// GitCommonDir resolves the bare repo root for a worktree at dir.
// Returns "" if dir is not inside a bare repo worktree.
func GitCommonDir(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(string(out))
	if gitDir == "" || gitDir == ".git" {
		return ""
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	abs, err := filepath.Abs(gitDir)
	if err != nil {
		return ""
	}
	return abs
}

func BareRepoWorktrees(bare string, skipDefault bool) []string {
	worktreesDir := filepath.Join(bare, "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if skipDefault && (e.Name() == "main" || e.Name() == "master") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(worktreesDir, e.Name(), "gitdir"))
		if err != nil {
			continue
		}
		out = append(out, filepath.Dir(strings.TrimSpace(string(data))))
	}
	return out
}

func ProjectDirs(dir string) []string {
	if !IsBareRepo(dir) {
		return []string{dir}
	}
	return BareRepoWorktrees(dir, false)
}

func FeatureWorktrees(dir string) []string {
	if !IsBareRepo(dir) {
		return nil
	}
	return BareRepoWorktrees(dir, true)
}

func ScanReposParallel(roots []string, fn func(string) []string) []string {
	var subdirs []string
	for _, root := range roots {
		if !IsDir(root) {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				subdirs = append(subdirs, filepath.Join(root, e.Name()))
			}
		}
	}

	var mu sync.Mutex
	var out []string
	var wg sync.WaitGroup
	for _, p := range subdirs {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			r := fn(p)
			mu.Lock()
			out = append(out, r...)
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return out
}

func ExpandPaths(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = os.ExpandEnv(p)
	}
	return out
}

func IsDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func TrimSlash(s string) string {
	return strings.TrimRight(s, "/")
}
