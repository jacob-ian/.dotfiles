package pr

import (
	"flag"
	"fmt"
	"strings"

	"jmux/internal/ghctl"
)

// RunPreview prints the highlighted PR's body and threads, invoked by fzf on
// every cursor move. The current-repo picker passes `--dir D <#num line>`; the
// cross-repo assigned picker passes `--global <owner/repo#num line>`.
func RunPreview(args []string) {
	fs := flag.NewFlagSet("pr preview", flag.ContinueOnError)
	dir := fs.String("dir", "", "Directory to run gh in")
	global := fs.Bool("global", false, "Parse owner/repo#num and view by --repo")
	if err := fs.Parse(args); err != nil {
		return
	}
	line := strings.Join(fs.Args(), " ")

	if *global {
		slug, num, ok := parseRepoNumber(line)
		if !ok {
			return
		}
		fmt.Print(ghctl.ViewRepo(slug, num))
		return
	}

	num, ok := ParseNumber(line)
	if !ok {
		return
	}
	fmt.Print(ghctl.View(*dir, num))
}
