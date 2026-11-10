package workspace

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"jmux/internal/claudectl"
	"jmux/internal/gitctl"
	"jmux/internal/repo"
	"jmux/internal/tag"
	"jmux/internal/worktree"
)

const spawnUsage = `usage: jmux spawn [--repo <dir|name>] <branch> [prompt]
       jmux spawn list

Creates a worktree for <branch> and starts claude in it on the prompt,
in a new tmux session that is not switched to. Prints the workspace path.
  --repo   a path inside the repo, or a scanned repo's name;
           defaults to the repo containing the current directory
  prompt   one quoted argument; omit it to read the prompt from stdin
  list     the workspaces this claude session has spawned, one per line as
           "<path>\t<status>", status being working, needs_input, idle or empty
Flags go before <branch>. Open the result with: jmux open <path>`

const (
	spawnKind    = "spawn"
	sessionIDEnv = "CLAUDE_CODE_SESSION_ID"
)

type spawnData struct {
	Parent string    `json:"parent"`
	At     time.Time `json:"at"`
}

// RunSpawn handles `jmux spawn`; stdout is the new workspace's path.
func RunSpawn(cfg repo.Config, args []string) error {
	fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repoRef := fs.String("repo", "", "path or name of the repo")
	if err := fs.Parse(args); err != nil || fs.NArg() < 1 || fs.NArg() > 2 {
		return errors.New(spawnUsage)
	}
	if fs.Arg(0) == "list" {
		return listSpawns(os.Stdout, os.Getenv(sessionIDEnv))
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
	if parent := os.Getenv(sessionIDEnv); parent != "" {
		tag.Set(path, spawnKind, tag.New(spawnKind, "", spawnData{Parent: parent, At: time.Now()}))
	}
	fmt.Println(path)
	return nil
}

// listSpawns writes the workspaces spawned by the claude session parent and
// the status word of the claude in each, oldest first.
func listSpawns(w io.Writer, parent string) error {
	if parent == "" {
		return errors.New("not running under claude: " + sessionIDEnv + " is unset")
	}
	type spawn struct {
		path string
		at   time.Time
	}
	var mine []spawn
	for path, t := range tag.Namespace(spawnKind) {
		var d spawnData
		if json.Unmarshal(t.Data, &d) == nil && d.Parent == parent && repo.IsDir(path) {
			mine = append(mine, spawn{path, d.At})
		}
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i].at.Before(mine[j].at) })
	for _, s := range mine {
		fmt.Fprintf(w, "%s\t%s\n", s.path, claudectl.Status(s.path))
	}
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
