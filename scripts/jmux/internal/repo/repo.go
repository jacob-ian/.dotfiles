package repo

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config is where jmux looks for repos. Paths are absolute; roots that don't
// exist on a machine are skipped, so one Default serves every machine.
type Config struct {
	// ScanRoots are the parent directories under which repos and worktrees live.
	ScanRoots []string
	// AdditionalDirs are non-repo directories included in the default picker.
	AdditionalDirs []string
	// ScratchRoot is the bare repo of dated scratch sessions (see package
	// scratch). It sits outside the scan roots so it works without them.
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

// Marker is the picker prefix for scratch rows; other rows get the same width
// so paths stay aligned.
func (c Config) Marker(path string) string {
	if c.IsScratch(path) {
		return "✎ "
	}
	return "  "
}

// WorkDir is where a session's windows start: a scratch worktree's dated
// directory (worktree/<name>, the session's own files) when it exists,
// otherwise dir itself.
func (c Config) WorkDir(dir string) string {
	if c.IsScratch(dir) {
		if nested := filepath.Join(dir, filepath.Base(dir)); IsDir(nested) {
			return nested
		}
	}
	return dir
}

// BareRepos returns the bare-repo roots found directly under the scan roots.
func (c Config) BareRepos() []string {
	return ScanReposParallel(c.ScanRoots, func(dir string) []string {
		if isBareRepo(dir) {
			return []string{dir}
		}
		return nil
	})
}

// FeatureWorktrees lists every non-default worktree across the scan roots and
// the scratch repo.
func (c Config) FeatureWorktrees() []string {
	out := ScanReposParallel(c.ScanRoots, FeatureWorktrees)
	return append(out, FeatureWorktrees(c.ScratchRoot)...)
}

// ProjectDirs lists everything the default picker offers: every worktree under
// the scan roots, the additional dirs and roots themselves, and the scratch
// repo's worktrees.
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

// DefaultBranchName reads bare's HEAD file directly (this package stays
// git-free); "" when missing or detached.
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

// BareRepoWorktrees lists bare's working trees; skipDefault drops the checkout
// named by HEAD, and main/master regardless (some bare HEADs point at a branch
// that was never checked out, e.g. develop, with master as the real default).
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

// Resolve returns the absolute, symlink-resolved form of p, so paths that name
// the same directory compare equal. A path that doesn't exist yet resolves its
// nearest existing ancestor and keeps the rest, so a to-be-created worktree
// compares equal to its eventual real path.
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
