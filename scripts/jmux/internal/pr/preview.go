package pr

import (
	"fmt"
	"strings"

	"jmux/internal/ghctl"
)

// RunPreview prints the highlighted PR's body + threads for fzf. The row is
// "owner/repo#num …"; the repo and number are parsed straight from it.
func RunPreview(args []string) {
	slug, num, ok := parseRepoNumber(strings.Join(args, " "))
	if !ok {
		return
	}
	fmt.Print(ghctl.ViewRepo(slug, num))
}
