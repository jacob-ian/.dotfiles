// Package nvimctl controls Neovim processes.
package nvimctl

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// WindowName is the tmux window the editor runs in. It's the contract between
// opening a session (which creates the window) and anything that finds the
// editor later (e.g. the workspace preview).
const WindowName = "nvim"

// Processes returns the PIDs of the Neovim editor process trees anchored under
// the given root PIDs (e.g. tmux pane PIDs): each editor reachable from a root,
// plus its descendants (the language servers it spawned). Call before the roots
// are killed, while the trees are still attached.
//
// Neovim 0.12 runs the editor as a detached `nvim --embed` process in its own
// group, so a kill aimed at a pane leaves it — and its language servers —
// behind; returning the whole tree lets callers reap it.
func Processes(roots []int) []int {
	if len(roots) == 0 {
		return nil
	}
	anchor := make(map[int]bool, len(roots))
	for _, r := range roots {
		anchor[r] = true
	}

	// One ps snapshot: pid↔ppid graph plus the embed cores.
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

	// Keep cores whose ancestry reaches an anchor (capped walk tolerates a shell
	// between anchor and TUI), and with each core its descendant LSPs.
	var reap []int
	seen := map[int]bool{}
	var collect func(pid int)
	collect = func(pid int) {
		if seen[pid] {
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

// Reap SIGKILLs the given PIDs. A Neovim editor ignores SIGTERM (it expects its
// UI to drive shutdown), so SIGKILL is what reaps it.
func Reap(pids []int) {
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
