package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

func main() {
	var selected string

	if len(os.Args) == 2 {
		selected = os.Args[1]
	} else {
		dirs := buildDirectoryList()
		if len(dirs) == 0 {
			os.Exit(0)
		}
		var err error
		selected, err = runFzf(dirs)
		if err != nil || selected == "" {
			os.Exit(0)
		}
	}

	selected = strings.TrimRight(selected, "/")
	if selected == "" {
		os.Exit(0)
	}

	parentDir := filepath.Base(filepath.Dir(selected))
	selectedDir := filepath.Base(selected)
	selectedName := strings.ReplaceAll(parentDir+"_"+selectedDir, ".", "_")

	openTmuxSession(selectedName, selected)
}

func buildDirectoryList() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	scanRoots := []string{
		filepath.Join(home, "dev"),
		filepath.Join(home, "euc"),
		filepath.Join(home, "net"),
	}

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

	// Add special directories
	results = append(results, filepath.Join(home, ".config"))

	for _, name := range []string{".cursor", ".claude"} {
		p := filepath.Join(home, name)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			results = append(results, p)
		}
	}

	// Add root directories themselves
	for _, d := range existingRoots {
		results = append(results, d)
	}

	return results
}

// scanRepo checks if a directory is a bare git repo.
// If bare: returns its worktrees (excluding the bare root).
// Otherwise: returns the directory itself.
func scanRepo(dir string) []string {
	// Check if bare repo
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-bare-repository")
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		// Not a bare repo (or not a git repo at all) — return the directory itself
		return []string{dir}
	}

	// It's a bare repo — list worktrees
	cmd = exec.Command("git", "-C", dir, "worktree", "list", "--porcelain")
	out, err = cmd.Output()
	if err != nil {
		return nil
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}
	// Resolve symlinks so comparison works reliably
	absDir, _ = filepath.EvalSymlinks(absDir)

	var worktrees []string
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		wt := strings.TrimPrefix(line, "worktree ")
		wtResolved, _ := filepath.EvalSymlinks(wt)
		if wtResolved == "" {
			wtResolved = wt
		}
		// Exclude the bare root itself
		if wtResolved == absDir {
			continue
		}
		worktrees = append(worktrees, wt)
	}

	return worktrees
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

func openTmuxSession(name, dir string) {
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
