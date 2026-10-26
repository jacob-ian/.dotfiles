package prompt_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"jmux/internal/prompt"
)

func TestLoad(t *testing.T) {
	t.Run("missing file is empty", func(t *testing.T) {
		c, err := prompt.Load(filepath.Join(t.TempDir(), "prompts.yaml"))
		if err != nil || len(c.Prompts) != 0 {
			t.Fatalf("got %+v, %v", c, err)
		}
	})
	t.Run("parses skills and prompts", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "prompts.yaml")
		os.WriteFile(p, []byte("skills:\n  go: [a, b]\nprompts:\n  implement: do it\n  review: look\n"), 0o644)
		c, err := prompt.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(c.Names(), []string{"implement", "review"}) {
			t.Errorf("names = %v", c.Names())
		}
		if !slices.Equal(c.Skills["go"], []string{"a", "b"}) {
			t.Errorf("skills = %v", c.Skills)
		}
	})
}

func TestLanguage(t *testing.T) {
	tests := map[string]struct {
		files   []string
		content map[string]string
		want    string
	}{
		"go":               {files: []string{"go.mod"}, want: "go"},
		"ts":               {files: []string{"package.json", "yarn.lock"}, want: "ts"},
		"ts backend":       {files: []string{"package.json"}, content: map[string]string{"package.json": `{"dependencies":{"slonik":"1"}}`}, want: "ts"},
		"react":            {files: []string{"package.json"}, content: map[string]string{"package.json": `{"dependencies":{"react":"19"}}`}, want: "react"},
		"react native dev": {files: []string{"package.json"}, content: map[string]string{"package.json": `{"devDependencies":{"react-native":"0.76"}}`}, want: "react"},
		"react in workspace": {
			files: []string{"package.json", "packages/apps/web/package.json"},
			content: map[string]string{
				"package.json":                   `{"workspaces":["packages/apps/*"]}`,
				"packages/apps/web/package.json": `{"dependencies":{"react":"19"}}`,
			},
			want: "react",
		},
		"workspace without react": {
			files: []string{"package.json", "packages/lib/package.json"},
			content: map[string]string{
				"package.json":              `{"workspaces":{"packages":["packages/*"]}}`,
				"packages/lib/package.json": `{"dependencies":{"zod":"3"}}`,
			},
			want: "ts",
		},
		"python":           {files: []string{"requirements.txt"}, want: "python"},
		"terraform":        {files: []string{"main.tf", "variables.tf"}, want: "terraform"},
		"nested terraform": {files: []string{"modules/dd/main.tf", "atlantis.yaml"}, want: "terraform"},
		"unknown":          {files: []string{"README.md"}, want: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755)
				os.WriteFile(filepath.Join(dir, f), []byte(tt.content[f]), 0o644)
			}
			if got := prompt.Language(dir); got != tt.want {
				t.Errorf("Language() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	c := prompt.Config{
		Skills: map[string][]string{"go": {"euc-go", "euc-sql", "missing"}, "common": {"euc-yaml", "absent"}},
		Prompts: map[string]string{
			"implement": "implement the ticket in {{.Repo}} ({{.Language}}). use {{.Skills}} and other relevant skills.",
			"plain":     "just do it",
		},
	}
	skills := t.TempDir()
	for _, s := range []string{"euc-go", "euc-sql", "euc-yaml"} {
		os.MkdirAll(filepath.Join(skills, s), 0o755)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), nil, 0o644)

	tests := map[string]struct {
		name    string
		want    string
		wantErr bool
	}{
		"installed skills only": {name: "implement", want: "implement the ticket in svc (go). use euc-go, euc-sql, euc-yaml and other relevant skills."},
		"no template vars":      {name: "plain", want: "just do it"},
		"unknown prompt":        {name: "nope", wantErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := c.Render(tt.name, "svc", dir, skills)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if got != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}
