package pr

import (
	"flag"
	"fmt"
	"strings"

	"jmux/internal/ghctl"
)

// RunPreview handles `jmux pr preview --dir D <line>`, printing the highlighted
// PR's body and threads. Invoked by fzf on every cursor move.
func RunPreview(args []string) {
	fs := flag.NewFlagSet("pr preview", flag.ContinueOnError)
	dir := fs.String("dir", "", "Directory to run gh in")
	if err := fs.Parse(args); err != nil {
		return
	}
	num, ok := ParseNumber(strings.Join(fs.Args(), " "))
	if !ok {
		return
	}
	fmt.Print(ghctl.View(*dir, num))
}
