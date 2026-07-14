package notify

import (
	"os/exec"
	"strconv"
	"strings"

	"jmux/internal/tmuxctl"
)

// This file locates the terminal emulator without naming one: every attached
// tmux client's process ancestry leads to the macOS app hosting it, so the
// terminal is whatever app that walk reaches.

// ActivateTerminal brings the app hosting an attached tmux client to the
// front, doing nothing when there is none to find.
func ActivateTerminal() {
	if id := terminalBundleID(); id != "" {
		exec.Command("open", "-b", id).Run()
	}
}

// ancestryDepth bounds pid walks; real chains are client←shell←login←app.
const ancestryDepth = 20

// terminalFocused reports whether the frontmost macOS app hosts an attached
// tmux client — the terminal is on screen, so the statusline multibox is
// already in view. False on any failure, failing toward delivering alerts.
func terminalFocused() bool {
	asn := ""
	if out, err := exec.Command("lsappinfo", "front").Output(); err == nil {
		asn = strings.TrimSpace(string(out))
	}
	front, err := strconv.Atoi(lsappinfoValue("info", "-only", "pid", asn))
	if err != nil || front <= 1 {
		return false
	}
	parents := processParents()
	for _, client := range tmuxctl.ClientPIDs() {
		for pid, depth := client, 0; pid > 1 && depth < ancestryDepth; pid, depth = parents[pid], depth+1 {
			if pid == front {
				return true
			}
		}
	}
	return false
}

// terminalBundleID returns the bundle id of the app hosting an attached tmux
// client, or "" when there is none to find.
func terminalBundleID() string {
	parents := processParents()
	for _, client := range tmuxctl.ClientPIDs() {
		for pid, depth := client, 0; pid > 1 && depth < ancestryDepth; pid, depth = parents[pid], depth+1 {
			if id := lsappinfoValue("info", "-only", "bundleid", strconv.Itoa(pid)); id != "" {
				return id
			}
		}
	}
	return ""
}

// processParents snapshots pid→ppid in one ps call for ancestry walks.
func processParents() map[int]int {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		return nil
	}
	m := map[int]int{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 == nil && err2 == nil {
			m[pid] = ppid
		}
	}
	return m
}

// lsappinfoValue runs lsappinfo and returns the value after '=', which is a
// quoted string or a bare number; anything else (notably `[ NULL ]` for
// processes that aren't apps) comes back "".
func lsappinfoValue(args ...string) string {
	out, err := exec.Command("lsappinfo", args...).Output()
	if err != nil {
		return ""
	}
	_, v, ok := strings.Cut(strings.TrimSpace(string(out)), "=")
	if !ok {
		return ""
	}
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, `"`) {
		return strings.Trim(v, `"`)
	}
	if _, err := strconv.Atoi(v); err == nil {
		return v
	}
	return ""
}
