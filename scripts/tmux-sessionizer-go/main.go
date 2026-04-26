package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	scanRoots      = []string{"$HOME/dev", "$HOME/euc", "$HOME/net"}
	additionalDirs = []string{"$HOME/.config", "$HOME/.claude"}
)

func main() {
	nameOnly := flag.Bool("name", false, "Print the session name for the given directory and exit")
	worktreesOnly := flag.Bool("worktrees", false, "Show only non-main/master worktrees")
	withClaude := flag.Bool("claude", false, "Create an additional window running `claude`")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [directory]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "Open a tmux session for a project directory.\n")
		fmt.Fprintf(os.Stderr, "If no directory is given, an fzf picker is shown.\n")
		fmt.Fprintf(os.Stderr, "  -worktrees  Show only non-main/master worktrees\n")
		fmt.Fprintf(os.Stderr, "  -claude     Create an additional window running `claude`\n")
		fmt.Fprintf(os.Stderr, "  -name       Print the session name for <directory> and exit\n")
	}
	flag.Parse()

	if *nameOnly {
		if flag.NArg() != 1 {
			flag.Usage()
			os.Exit(1)
		}
		fmt.Println(newSessionName(trimSlash(flag.Arg(0))))
		return
	}

	selected := pickDirectory(*worktreesOnly)
	if selected == "" {
		return
	}
	openSession(newSessionName(selected), selected, *withClaude)
}

func pickDirectory(worktreesOnly bool) string {
	if flag.NArg() == 1 && !worktreesOnly {
		return trimSlash(flag.Arg(0))
	}
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(1)
	}

	roots := expandPaths(scanRoots)
	var dirs []string
	if worktreesOnly {
		dirs = scanReposParallel(roots, featureWorktrees)
	} else {
		dirs = scanReposParallel(roots, projectDirs)
		for _, d := range expandPaths(additionalDirs) {
			if isDir(d) {
				dirs = append(dirs, d)
			}
		}
		for _, d := range roots {
			if isDir(d) {
				dirs = append(dirs, d)
			}
		}
	}
	if len(dirs) == 0 {
		return ""
	}
	sel, err := runFzf(dirs)
	if err != nil {
		return ""
	}
	return trimSlash(sel)
}

func newSessionName(dir string) string {
	if bareRoot := findBareRoot(dir); bareRoot != "" && bareRoot != dir {
		rel, err := filepath.Rel(bareRoot, dir)
		if err != nil {
			rel = filepath.Base(dir)
		}
		name := filepath.Base(bareRoot) + "_" + rel
		name = strings.ReplaceAll(name, "/", "_")
		return strings.ReplaceAll(name, ".", "_")
	}
	parent := filepath.Base(filepath.Dir(dir))
	base := filepath.Base(dir)
	return strings.ReplaceAll(parent+"_"+base, ".", "_")
}

func findBareRoot(dir string) string {
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

func expandPaths(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = os.ExpandEnv(p)
	}
	return out
}

func trimSlash(s string) string {
	return strings.TrimRight(s, "/")
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func scanReposParallel(roots []string, fn func(string) []string) []string {
	var subdirs []string
	for _, root := range roots {
		if !isDir(root) {
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

func projectDirs(dir string) []string {
	if !isBareRepo(dir) {
		return []string{dir}
	}
	return bareRepoWorktrees(dir, false)
}

func featureWorktrees(dir string) []string {
	if !isBareRepo(dir) {
		return nil
	}
	return bareRepoWorktrees(dir, true)
}

func bareRepoWorktrees(bare string, skipDefault bool) []string {
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

func runFzf(items []string) (string, error) {
	cmd := exec.Command("fzf")
	cmd.Stdin = strings.NewReader(strings.Join(items, "\n"))
	cmd.Stderr = os.Stderr
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func openSession(name, dir string, withClaude bool) {
	if err := exec.Command("tmux", "has-session", "-t="+name).Run(); err != nil {
		exec.Command("tmux", "new-session", "-ds", name, "-n", "nvim", "-c", dir, "nvim").Run()
		if withClaude {
			exec.Command("tmux", "new-window", "-t", name+":", "-n", "claude", "-c", dir, "claude").Run()
			exec.Command("tmux", "select-window", "-t", name+":1").Run()
		}
	}

	if os.Getenv("TMUX") != "" {
		exec.Command("tmux", "switch-client", "-t", name).Run()
		return
	}

	cmd := exec.Command("tmux", "attach", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}
