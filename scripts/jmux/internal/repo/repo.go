package repo

import (
	"os"
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

// AdminDirFor returns the path of the worktree admin entry under
// `<bare>/worktrees/` whose gitdir resolves to worktreePath. Returns ""
// if no matching admin entry is found.
func AdminDirFor(bare, worktreePath string) string {
	worktreesDir := filepath.Join(bare, "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return ""
	}
	want, err := filepath.Abs(worktreePath)
	if err != nil {
		want = worktreePath
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		adminDir := filepath.Join(worktreesDir, e.Name())
		data, err := os.ReadFile(filepath.Join(adminDir, "gitdir"))
		if err != nil {
			continue
		}
		got := filepath.Dir(strings.TrimSpace(string(data)))
		if abs, err := filepath.Abs(got); err == nil {
			got = abs
		}
		if got == want {
			return adminDir
		}
	}
	return ""
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
