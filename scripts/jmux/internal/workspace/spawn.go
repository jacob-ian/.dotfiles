package workspace

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"jmux/internal/gitctl"
	"jmux/internal/repo"
	"jmux/internal/worktree"
)

const spawnUsage = `usage: jmux spawn [--repo <dir|name>] <branch> [prompt]

Creates a worktree for <branch> and starts claude in it on the prompt,
in a new tmux session that is not switched to. Prints the workspace path.
  --repo   a path inside the repo, or a scanned repo's name;
           defaults to the repo containing the current directory
  prompt   one quoted argument; omit it to read the prompt from stdin
Flags go before <branch>. Open the result with: jmux open <path>`

// RunSpawn handles `jmux spawn`; stdout is the new workspace's path.
func RunSpawn(cfg repo.Config, args []string) error {
	fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repoRef := fs.String("repo", "", "path or name of the repo")
	if err := fs.Parse(args); err != nil || fs.NArg() < 1 || fs.NArg() > 2 {
		return errors.New(spawnUsage)
	}
	bareRoot, err := bareRootFor(cfg, *repoRef)
	if err != nil {
		return err
	}
	prompt, err := promptArg(fs.Args()[1:], os.Stdin)
	if err != nil {
		return err
	}
	path, err := worktree.Spawn(bareRoot, fs.Arg(0), prompt)
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func promptArg(rest []string, stdin io.Reader) (string, error) {
	if len(rest) == 1 {
		return rest[0], nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", errors.New(spawnUsage)
	}
	return prompt, nil
}

// bareRootFor resolves ref, a directory in or under a jmux repo or the name
// of a scanned bare repo, to its bare root. Empty ref means the cwd.
func bareRootFor(cfg repo.Config, ref string) (string, error) {
	if ref == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		if root := bareRootOf(cwd); root != "" {
			return root, nil
		}
		return "", errors.New("not inside a jmux repo; pass --repo")
	}
	if abs, err := filepath.Abs(ref); err == nil && repo.IsDir(abs) {
		if root := bareRootOf(abs); root != "" {
			return root, nil
		}
		return "", fmt.Errorf("%s is not a jmux repo", ref)
	}
	var matches []string
	for _, b := range cfg.BareRepos() {
		if b = repo.TrimSlash(b); filepath.Base(b) == ref {
			matches = append(matches, b)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("no repo named %s", ref)
	}
	return "", fmt.Errorf("%s is ambiguous: %s", ref, strings.Join(matches, ", "))
}

func bareRootOf(dir string) string {
	if root := gitctl.CommonDir(dir); root != "" {
		return root
	}
	return repo.FindBareRoot(dir)
}
