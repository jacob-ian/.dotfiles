package workspace

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jmux/internal/repo"
	"jmux/internal/tag"
)

func TestPromptArg(t *testing.T) {
	if got, _ := promptArg([]string{"do it"}, strings.NewReader("ignored")); got != "do it" {
		t.Fatalf("arg: got %q", got)
	}
	if got, _ := promptArg(nil, strings.NewReader("  from stdin\n")); got != "from stdin" {
		t.Fatalf("stdin: got %q", got)
	}
	if _, err := promptArg(nil, strings.NewReader("")); err == nil {
		t.Fatal("empty stdin: want error")
	}
}

func TestBareRootForName(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(root, name, "refs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := repo.Config{ScanRoots: []string{root}}
	got, err := bareRootFor(cfg, "beta")
	if err != nil || got != filepath.Join(root, "beta") {
		t.Fatalf("name: got %q, %v", got, err)
	}
	if got, err := bareRootFor(cfg, filepath.Join(root, "alpha")); err != nil || got != filepath.Join(root, "alpha") {
		t.Fatalf("path: got %q, %v", got, err)
	}
	if _, err := bareRootFor(cfg, "gamma"); err == nil {
		t.Fatal("unknown name: want error")
	}
	if _, err := bareRootFor(cfg, t.TempDir()); err == nil {
		t.Fatal("plain dir: want error")
	}
}

func TestListSpawnsFiltersByParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	mine, theirs := t.TempDir(), t.TempDir()
	tag.Set(mine, "spawn", tag.New("spawn", "", map[string]string{"parent": "me", "at": "2026-01-01T00:00:00Z"}))
	tag.Set(theirs, "spawn", tag.New("spawn", "", map[string]string{"parent": "other", "at": "2026-01-01T00:00:00Z"}))

	var out bytes.Buffer
	if err := listSpawns(&out, "me"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.HasPrefix(got, repo.Resolve(mine)+"\t") || strings.Contains(got, theirs) {
		t.Fatalf("got %q", got)
	}
	if err := listSpawns(&out, ""); err == nil {
		t.Fatal("no session id: want error")
	}
}
