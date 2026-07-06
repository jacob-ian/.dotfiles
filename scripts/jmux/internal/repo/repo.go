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

// isBareRepo reports whether dir is a bare repo root (HEAD + refs/, no .git).
func isBareRepo(dir string) bool {
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
		if isBareRepo(cur) {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

// adminEntry pairs a worktree admin directory under `<bare>/worktrees/<name>`
// with the resolved working tree directory it points at (read from `gitdir`).
type adminEntry struct {
	adminDir string
	gitdir   string
}

// readAdmins enumerates worktree admin entries under `<bare>/worktrees/`,
// skipping any that lack a readable `gitdir` pointer file.
func readAdmins(bare string) []adminEntry {
	worktreesDir := filepath.Join(bare, "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return nil
	}
	var out []adminEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		adminDir := filepath.Join(worktreesDir, e.Name())
		data, err := os.ReadFile(filepath.Join(adminDir, "gitdir"))
		if err != nil {
			continue
		}
		out = append(out, adminEntry{
			adminDir: adminDir,
			gitdir:   filepath.Dir(strings.TrimSpace(string(data))),
		})
	}
	return out
}

// AdminDirFor returns the path of the worktree admin entry under
// `<bare>/worktrees/` whose gitdir resolves to worktreePath. Returns ""
// if no matching admin entry is found.
func AdminDirFor(bare, worktreePath string) string {
	want, err := filepath.Abs(worktreePath)
	if err != nil {
		want = worktreePath
	}
	for _, a := range readAdmins(bare) {
		got := a.gitdir
		if abs, err := filepath.Abs(got); err == nil {
			got = abs
		}
		if got == want {
			return a.adminDir
		}
	}
	return ""
}

func BareRepoWorktrees(bare string, skipDefault bool) []string {
	var out []string
	for _, a := range readAdmins(bare) {
		if skipDefault {
			name := filepath.Base(a.adminDir)
			if name == "main" || name == "master" {
				continue
			}
		}
		out = append(out, a.gitdir)
	}
	return out
}

// BareRepos returns the bare-repo roots found directly under the scan roots.
func BareRepos() []string {
	roots := ExpandPaths(ScanRoots)
	return ScanReposParallel(roots, func(dir string) []string {
		if isBareRepo(dir) {
			return []string{dir}
		}
		return nil
	})
}

func ProjectDirs(dir string) []string {
	if !isBareRepo(dir) {
		return []string{dir}
	}
	return BareRepoWorktrees(dir, false)
}

func FeatureWorktrees(dir string) []string {
	if !isBareRepo(dir) {
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
		wg.Go(func() {
			r := fn(p)
			mu.Lock()
			out = append(out, r...)
			mu.Unlock()
		})
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

// Resolve returns the absolute, symlink-resolved form of p, so paths that name
// the same directory compare equal. Falls back to the best form it could make.
func Resolve(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}
