package generator

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hay-kot/mmdot/internal/core"
)

func TestBrewfilePartial(t *testing.T) {
	dir := t.TempDir()
	outfile := filepath.Join(dir, "brew.sh")

	cfg := &core.ConfigFile{
		Brews: core.ConfigMap{
			"base": &core.Brews{
				Brews: []string{"curl", "wget"},
			},
			"personal": &core.Brews{
				Includes: []string{"base"},
				Taps:     []string{"homebrew/cask"},
				Brews:    []string{"git", "vim"},
				Casks:    []string{"firefox"},
				MAS:      []string{"497799835"},
			},
		},
		Variables: core.Variables{},
	}

	engine := NewEngine(cfg)

	tmpl := core.Template{
		Name:   "test-brewfile",
		Output: outfile,
		Template: `#!/bin/bash
set -euo pipefail
{{template "brewfile" "personal"}}`,
	}

	err := engine.RenderTemplate(context.Background(), tmpl)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	got, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	output := string(got)

	// Verify includes are resolved (base brews merged in)
	for _, want := range []string{
		"brew tap homebrew/cask",
		"brew install curl",
		"brew install wget",
		"brew install git",
		"brew install vim",
		"brew install --cask firefox",
		"mas install 497799835",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("output missing %q\n\ngot:\n%s", want, output)
		}
	}
}

func TestBrewfilePartialRemove(t *testing.T) {
	dir := t.TempDir()
	outfile := filepath.Join(dir, "brew-remove.sh")

	cfg := &core.ConfigFile{
		Brews: core.ConfigMap{
			"cleanup": &core.Brews{
				Remove: true,
				Taps:   []string{"old/tap"},
				Brews:  []string{"oldpkg"},
				Casks:  []string{"oldcask"},
				MAS:    []string{"123456"},
			},
		},
		Variables: core.Variables{},
	}

	engine := NewEngine(cfg)

	tmpl := core.Template{
		Name:     "test-remove",
		Output:   outfile,
		Template: `{{template "brewfile" "cleanup"}}`,
	}

	err := engine.RenderTemplate(context.Background(), tmpl)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	got, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	output := string(got)

	for _, want := range []string{
		"brew untap old/tap",
		"brew uninstall oldpkg",
		"brew uninstall --cask oldcask",
		"mas uninstall 123456",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("output missing %q\n\ngot:\n%s", want, output)
		}
	}
}

func TestBrewfilePartialUnknownConfig(t *testing.T) {
	dir := t.TempDir()
	outfile := filepath.Join(dir, "out.sh")

	cfg := &core.ConfigFile{
		Brews:     core.ConfigMap{},
		Variables: core.Variables{},
	}

	engine := NewEngine(cfg)

	tmpl := core.Template{
		Name:     "test-unknown",
		Output:   outfile,
		Template: `{{template "brewfile" "nonexistent"}}`,
	}

	err := engine.RenderTemplate(context.Background(), tmpl)
	if err == nil {
		t.Fatal("expected error for unknown brew config, got nil")
	}
}
