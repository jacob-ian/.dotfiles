package repo

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config is where jmux looks for repos; roots missing on a machine are skipped.
type Config struct {
	ScanRoots      []string
	AdditionalDirs []string
	// ScratchRoot is the bare repo of dated scratch sessions.
	ScratchRoot string
}

func Default() Config {
	return Config{
		ScanRoots:      ExpandPaths([]string{"$HOME/dev", "$HOME/euc", "$HOME/net"}),
		AdditionalDirs: ExpandPaths([]string{"$HOME/.config", "$HOME/.claude"}),
		ScratchRoot:    os.ExpandEnv("$HOME/scratch"),
	}
}

func (c Config) IsScratch(path string) bool {
	root := Resolve(c.ScratchRoot)
	p := Resolve(path)
	return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
}

// Marker is the picker prefix for scratch rows, padded on other rows so paths align.
func (c Config) Marker(path string) string {
	if c.IsScratch(path) {
		return "✎ "
	}
	return "  "
}

// WorkDir is where a session's windows start: a scratch worktree's nested dated
// directory when it exists, else dir.
func (c Config) WorkDir(dir string) string {
	if c.IsScratch(dir) {
		if nested := filepath.Join(dir, filepath.Base(dir)); IsDir(nested) {
			return nested
		}
	}
	return dir
}

func (c Config) BareRepos() []string {
	return ScanReposParallel(c.ScanRoots, func(dir string) []string {
		if isBareRepo(dir) {
			return []string{dir}
		}
		return nil
	})
}

func (c Config) FeatureWorktrees() []string {
	out := ScanReposParallel(c.ScanRoots, FeatureWorktrees)
	return append(out, FeatureWorktrees(c.ScratchRoot)...)
}

// ProjectDirs lists everything the default picker offers.
func (c Config) ProjectDirs() []string {
	dirs := ScanReposParallel(c.ScanRoots, ProjectDirs)
	for _, d := range append(append([]string{}, c.AdditionalDirs...), c.ScanRoots...) {
		if IsDir(d) {
			dirs = append(dirs, d)
		}
	}
	if IsDir(c.ScratchRoot) {
		dirs = append(dirs, ProjectDirs(c.ScratchRoot)...)
	}
	return dirs
}

// DefaultBranchName reads bare's HEAD file; "" when missing or detached.
func DefaultBranchName(bare string) string {
	data, err := os.ReadFile(filepath.Join(bare, "HEAD"))
	if err != nil {
		return ""
	}
	ref, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "ref: refs/heads/")
	if !ok {
		return ""
	}
	return ref
}

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

// WorkRoot returns the working tree containing dir, or "" outside any repo.
// A pure walk so the claude hook doesn't spawn git on every event.
func WorkRoot(dir string) string {
	for cur := dir; ; {
		if _, err := os.Lstat(filepath.Join(cur, ".git")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

type adminEntry struct {
	adminDir string
	gitdir   string
}

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

// AdminDirFor returns the `<bare>/worktrees/<name>` entry for worktreePath, or "".
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

// BareRepoWorktrees lists bare's working trees. skipDefault drops the HEAD
// branch and also main/master, since some bare HEADs name a branch that was
// never checked out.
func BareRepoWorktrees(bare string, skipDefault bool) []string {
	def := DefaultBranchName(bare)
	var out []string
	for _, a := range readAdmins(bare) {
		if skipDefault {
			name := filepath.Base(a.adminDir)
			if name == def || name == "main" || name == "master" {
				continue
			}
		}
		out = append(out, a.gitdir)
	}
	return out
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

// Resolve returns the absolute, symlink-resolved form of p. A path that doesn't
// exist yet resolves through its nearest existing ancestor.
func Resolve(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	rest := ""
	for cur := abs; ; {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}
