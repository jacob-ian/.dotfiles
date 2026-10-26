// Package prompt renders the preset opening prompts offered when a workspace
// is created, from ~/.config/jmux/prompts.yaml.
package prompt

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"jmux/internal/fzfutil"
)

// Config is the prompts file: skills per language (the "common" entry joins
// every language), and named prompt templates that can reference .Repo,
// .Language and .Skills.
type Config struct {
	Skills  map[string][]string `yaml:"skills"`
	Prompts map[string]string   `yaml:"prompts"`
}

// Path is the prompts file, in the dotfiles so it follows the user between
// machines; skills that aren't installed on a machine are dropped at render.
func Path() string {
	return os.ExpandEnv("$HOME/.config/jmux/prompts.yaml")
}

// SkillsDir is where claude keeps user skills, one directory per skill.
func SkillsDir() string {
	return os.ExpandEnv("$HOME/.claude/skills")
}

// Load reads the prompts file; a missing file is an empty Config, not an error.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Names lists the prompt names in a stable order.
func (c Config) Names() []string {
	names := make([]string, 0, len(c.Prompts))
	for n := range c.Prompts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Pick offers the prompts with "none" first, so a bare enter skips. Returns
// "" when nothing was chosen.
func Pick(c Config) string {
	items := append([]string{"none"}, c.Names()...)
	sel, err := fzfutil.Pick(items, fzfutil.Options{Prompt: "prompt> "})
	if err != nil || sel == "" || sel == "none" {
		return ""
	}
	return sel
}

// Language guesses the worktree's primary language from its manifest files.
// A package.json depending on react or react-native is "react" rather than
// "ts", so frontend skills only load for frontend repos.
func Language(dir string) string {
	if usesReact(filepath.Join(dir, "package.json")) {
		return "react"
	}
	for _, l := range []struct {
		name    string
		markers []string
	}{
		{"go", []string{"go.mod"}},
		{"ts", []string{"package.json"}},
		{"python", []string{"pyproject.toml", "requirements.txt"}},
		// infra repos keep .tf files under modules/ and projects/, not the root.
		{"terraform", []string{"*.tf", "*/*.tf", "*/*/*.tf"}},
	} {
		for _, m := range l.markers {
			if hits, _ := filepath.Glob(filepath.Join(dir, m)); len(hits) > 0 {
				return l.name
			}
		}
	}
	return ""
}

// usesReact reports whether packageJSON, or any workspace package it declares
// (monorepos keep react in packages/*, not the root), depends on react or
// react-native.
func usesReact(packageJSON string) bool {
	data, err := os.ReadFile(packageJSON)
	if err != nil {
		return false
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
		Workspaces      json.RawMessage   `json:"workspaces"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	for _, deps := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
		if _, ok := deps["react"]; ok {
			return true
		}
		if _, ok := deps["react-native"]; ok {
			return true
		}
	}
	for _, pattern := range workspaceGlobs(pkg.Workspaces) {
		hits, _ := filepath.Glob(filepath.Join(filepath.Dir(packageJSON), pattern, "package.json"))
		for _, h := range hits {
			if usesReact(h) {
				return true
			}
		}
	}
	return false
}

// workspaceGlobs reads package.json's workspaces field in either of its forms:
// a list of globs, or {"packages": [...]}.
func workspaceGlobs(raw json.RawMessage) []string {
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var obj struct {
		Packages []string `json:"packages"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Packages
	}
	return nil
}

// Data is what a prompt template sees.
type Data struct {
	Repo     string
	Language string
	// Skills is the language's list, filtered to those installed, joined with
	// ", " for use in prose.
	Skills string
}

// Render fills the named prompt for the worktree at dir. Skills missing from
// skillsDir are dropped so one prompts file serves machines with different
// skill sets.
func (c Config) Render(name, repo, dir, skillsDir string) (string, error) {
	text, ok := c.Prompts[name]
	if !ok {
		return "", errors.New("unknown prompt " + name)
	}
	lang := Language(dir)
	var skills []string
	for _, s := range append(append([]string{}, c.Skills[lang]...), c.Skills["common"]...) {
		if _, err := os.Stat(filepath.Join(skillsDir, s)); err == nil {
			skills = append(skills, s)
		}
	}
	tmpl, err := template.New(name).Parse(text)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	err = tmpl.Execute(&out, Data{Repo: repo, Language: lang, Skills: strings.Join(skills, ", ")})
	return strings.TrimSpace(out.String()), err
}
