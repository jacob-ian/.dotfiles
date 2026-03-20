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
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [directory]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "Open a tmux session for a project directory.\n")
		fmt.Fprintf(os.Stderr, "If no directory is given, an fzf picker is shown.\n")
	}
	flag.Parse()

	var selected string

	if flag.NArg() == 1 {
		selected = flag.Arg(0)
	} else if flag.NArg() == 0 {
		roots := expandPaths(scanRoots)
		additional := expandPaths(additionalDirs)

		dirs := buildDirectoryList(roots, additional)
		if len(dirs) == 0 {
			os.Exit(0)
		}
		var err error
		selected, err = runFzf(dirs)
		if err != nil || selected == "" {
			os.Exit(0)
		}
	} else {
		flag.Usage()
		os.Exit(1)
	}

	selected = strings.TrimRight(selected, "/")
	if selected == "" {
		os.Exit(0)
	}

	openSession(newSessionName(selected), selected)
}

func newSessionName(dir string) string {
	parent := filepath.Base(filepath.Dir(dir))
	base := filepath.Base(dir)
	return strings.ReplaceAll(parent+"_"+base, ".", "_")
}

func expandPaths(paths []string) []string {
	expanded := make([]string, len(paths))
	for i, p := range paths {
		expanded[i] = os.ExpandEnv(p)
	}
	return expanded
}

func buildDirectoryList(scanRoots, additionalDirs []string) []string {
	// Filter to existing directories
	var existingRoots []string
	for _, d := range scanRoots {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			existingRoots = append(existingRoots, d)
		}
	}

	// Collect subdirectories to scan
	type repoJob struct {
		path string
	}
	var jobs []repoJob
	for _, root := range existingRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				jobs = append(jobs, repoJob{path: filepath.Join(root, e.Name())})
			}
		}
	}

	// Scan all repos concurrently
	var mu sync.Mutex
	var results []string
	var wg sync.WaitGroup

	for _, job := range jobs {
		wg.Add(1)
		go func(j repoJob) {
			defer wg.Done()
			dirs := scanRepo(j.path)
			mu.Lock()
			results = append(results, dirs...)
			mu.Unlock()
		}(job)
	}
	wg.Wait()

	// Add additional directories
	for _, d := range additionalDirs {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			results = append(results, d)
		}
	}

	results = append(results, existingRoots...)
	return results
}

// scanRepo checks if a directory is a bare git repo.
// If bare: returns its worktrees (excluding the bare root).
// Otherwise: returns the directory itself.
func scanRepo(dir string) []string {
	if !isBareRepo(dir) {
		return []string{dir}
	}

	worktreesDir := filepath.Join(dir, "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return nil
	}

	var worktrees []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		gitdirFile := filepath.Join(worktreesDir, e.Name(), "gitdir")
		data, err := os.ReadFile(gitdirFile)
		if err != nil {
			continue
		}
		wtPath := filepath.Dir(strings.TrimSpace(string(data)))
		worktrees = append(worktrees, wtPath)
	}

	return worktrees
}

// isBareRepo detects a bare git repo by checking for HEAD and refs/
// at the top level without a .git subdirectory.
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
	input := strings.Join(items, "\n")
	cmd := exec.Command("fzf")
	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = os.Stderr

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

func openSession(name, dir string) {
	tmuxEnv := os.Getenv("TMUX")

	// Check if tmux is running at all
	tmuxRunning := false
	if err := exec.Command("pgrep", "tmux").Run(); err == nil {
		tmuxRunning = true
	}

	if tmuxEnv == "" && !tmuxRunning {
		// Not in tmux, tmux not running — create and attach
		cmd := exec.Command("tmux", "new-session", "-s", name, "-c", dir, "nvim")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
		return
	}

	// Create session if it doesn't exist
	if err := exec.Command("tmux", "has-session", "-t="+name).Run(); err != nil {
		exec.Command("tmux", "new-session", "-ds", name, "-c", dir, "nvim").Run()
	}

	if tmuxEnv == "" {
		// Not in tmux — attach
		cmd := exec.Command("tmux", "attach", "-t", name)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	} else {
		// In tmux — switch client
		exec.Command("tmux", "switch-client", "-t", name).Run()
	}
}
