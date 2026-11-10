package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jmux/internal/repo"
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
