package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestAgeFile_YAMLParsing(t *testing.T) {
	input := `
recipients:
  - age1abc123
identity_file: ~/.age/key
files:
  - src: secrets/file.md.age
    dest: output/file.md
  - src: secrets/config.yml.age
    dest: .config/work.yml
    perm: "0600"
`
	var age Age
	if err := yaml.Unmarshal([]byte(input), &age); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(age.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(age.Files))
	}

	if age.Files[0].Src != "secrets/file.md.age" {
		t.Errorf("files[0].Src = %q, want %q", age.Files[0].Src, "secrets/file.md.age")
	}
	if age.Files[0].Dest != "output/file.md" {
		t.Errorf("files[0].Dest = %q, want %q", age.Files[0].Dest, "output/file.md")
	}
	if age.Files[0].Permissions != "" {
		t.Errorf("files[0].Permissions = %q, want empty", age.Files[0].Permissions)
	}

	if age.Files[1].Permissions != "0600" {
		t.Errorf("files[1].Permissions = %q, want %q", age.Files[1].Permissions, "0600")
	}
}

func TestResolvePaths_AgeFiles(t *testing.T) {
	cfg := &ConfigFile{
		Age: Age{
			Files: []AgeFile{
				{Src: "secrets/file.age", Dest: "output/file.md"},
				{Src: "secrets/other.age", Dest: "~/docs/other.md"},
			},
		},
	}

	pr := PathResolver{configDir: "/config/dir"}
	if err := cfg.resolvePaths(pr); err != nil {
		t.Fatalf("resolvePaths() error: %v", err)
	}

	if cfg.Age.Files[0].Src != "/config/dir/secrets/file.age" {
		t.Errorf("files[0].Src = %q, want %q", cfg.Age.Files[0].Src, "/config/dir/secrets/file.age")
	}
	if cfg.Age.Files[0].Dest != "/config/dir/output/file.md" {
		t.Errorf("files[0].Dest = %q, want %q", cfg.Age.Files[0].Dest, "/config/dir/output/file.md")
	}

	homeDir, _ := os.UserHomeDir()
	wantDest := filepath.Join(homeDir, "docs/other.md")
	if cfg.Age.Files[1].Dest != wantDest {
		t.Errorf("files[1].Dest = %q, want %q", cfg.Age.Files[1].Dest, wantDest)
	}
}
