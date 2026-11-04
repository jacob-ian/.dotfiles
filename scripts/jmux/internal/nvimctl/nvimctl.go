// Package nvimctl controls Neovim processes.
package nvimctl

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// WindowName is the tmux window the editor runs in.
const WindowName = "nvim"

// Processes returns the editor process trees under the given root PIDs, each
// editor plus its language servers. Neovim 0.12 runs the editor as a detached
// `nvim --embed` in its own group, so killing a pane leaves it behind.
func Processes(roots []int) []int {
	if len(roots) == 0 {
		return nil
	}
	anchor := make(map[int]bool, len(roots))
	for _, r := range roots {
		anchor[r] = true
	}

	ps, err := exec.Command("ps", "-axo", "pid=,ppid=,command=").Output()
	if err != nil {
		return nil
	}
	parent := map[int]int{}
	children := map[int][]int{}
	var candidates []int
	for line := range strings.SplitSeq(string(ps), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		parent[pid] = ppid
		children[ppid] = append(children[ppid], pid)
		if strings.HasPrefix(strings.Join(fields[2:], " "), "nvim --embed") {
			candidates = append(candidates, pid)
		}
	}

	var reap []int
	// Never reap our own subtree: a remove run from inside the editor descends
	// from the embed it's reaping.
	self := os.Getpid()
	seen := map[int]bool{}
	var collect func(pid int)
	collect = func(pid int) {
		if seen[pid] || pid == self {
			return
		}
		seen[pid] = true
		reap = append(reap, pid)
		for _, c := range children[pid] {
			collect(c)
		}
	}
	for _, pid := range candidates {
		for p, hops := parent[pid], 0; p > 1 && hops < 32; p, hops = parent[p], hops+1 {
			if anchor[p] {
				collect(pid)
				break
			}
		}
	}
	return reap
}

// Reap SIGKILLs the given PIDs; a Neovim editor ignores SIGTERM.
func Reap(pids []int) {
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
