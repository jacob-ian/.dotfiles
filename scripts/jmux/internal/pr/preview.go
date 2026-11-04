package pr

import (
	"fmt"
	"strings"

	"jmux/internal/ghctl"
)

// RunPreview prints the highlighted PR's body and threads for fzf.
func RunPreview(args []string) {
	slug, num, ok := parseRepoNumber(strings.Join(args, " "))
	if !ok {
		return
	}
	fmt.Print(ghctl.ViewRepo(slug, num))
}
