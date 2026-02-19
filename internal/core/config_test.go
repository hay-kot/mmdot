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

func TestParseOctalPermissions(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    os.FileMode
		wantErr bool
	}{
		{name: "0644", input: "0644", want: 0o644},
		{name: "0600", input: "0600", want: 0o600},
		{name: "0755", input: "0755", want: 0o755},
		{name: "no leading zero", input: "644", want: 0o644},
		{name: "invalid", input: "not-octal", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOctalPermissions(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseOctalPermissions(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseOctalPermissions(%q) = %o, want %o", tt.input, got, tt.want)
			}
		})
	}
}

func TestAgeFile_Validate(t *testing.T) {
	tests := []struct {
		name    string
		af      AgeFile
		wantErr bool
	}{
		{
			name: "valid",
			af:   AgeFile{Src: "a.age", Dest: "a.txt"},
		},
		{
			name: "valid with permissions",
			af:   AgeFile{Src: "a.age", Dest: "a.txt", Permissions: "0600"},
		},
		{
			name:    "empty src",
			af:      AgeFile{Src: "", Dest: "a.txt"},
			wantErr: true,
		},
		{
			name:    "empty dest",
			af:      AgeFile{Src: "a.age", Dest: ""},
			wantErr: true,
		},
		{
			name:    "src equals dest",
			af:      AgeFile{Src: "same", Dest: "same"},
			wantErr: true,
		},
		{
			name:    "invalid permissions",
			af:      AgeFile{Src: "a.age", Dest: "a.txt", Permissions: "garbage"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.af.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolvePaths_AgeFiles_ValidationFailure(t *testing.T) {
	cfg := &ConfigFile{
		Age: Age{
			Files: []AgeFile{
				{Src: "", Dest: "output/file.md"},
			},
		},
	}

	pr := PathResolver{configDir: "/config/dir"}
	if err := cfg.resolvePaths(pr); err == nil {
		t.Fatal("resolvePaths() expected error for invalid AgeFile, got nil")
	}
}
