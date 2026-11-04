package notify

import (
	"os/exec"
	"strconv"
	"strings"

	"jmux/internal/tmuxctl"
)

// ActivateTerminal brings the app hosting an attached tmux client to the front.
func ActivateTerminal() {
	if id := terminalBundleID(); id != "" {
		exec.Command("open", "-b", id).Run()
	}
}

// ancestryDepth bounds pid walks; real chains are client←shell←login←app.
const ancestryDepth = 20

// terminalFocused reports whether the frontmost app hosts a tmux client.
// False on any failure, failing toward delivering alerts.
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

// terminalBundleID is the bundle id of the app hosting an attached tmux client, or "".
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

// lsappinfoValue returns the value after '='; `[ NULL ]` (non-app processes) comes back "".
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
