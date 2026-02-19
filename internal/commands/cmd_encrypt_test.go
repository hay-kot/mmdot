package commands

import (
	"os"
	"testing"
)

func Test_parsePermissions(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    os.FileMode
		wantErr bool
	}{
		{name: "0644", input: "0644", want: 0o644},
		{name: "0600", input: "0600", want: 0o600},
		{name: "0755", input: "0755", want: 0o755},
		{name: "invalid", input: "not-octal", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePermissions(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePermissions(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parsePermissions(%q) = %o, want %o", tt.input, got, tt.want)
			}
		})
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("warning: failed to restore working directory: %v", err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir to %s: %v", dir, err)
	}
}

func Test_ensureGitignored(t *testing.T) {
	tmpDir := t.TempDir()
	chdir(t, tmpDir)

	path := "output/secret.md"

	// First call should create .gitignore and add the path
	if err := ensureGitignored(path); err != nil {
		t.Fatalf("first ensureGitignored() error: %v", err)
	}

	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if string(data) != path+"\n" {
		t.Errorf(".gitignore content = %q, want %q", string(data), path+"\n")
	}

	// Second call should be a no-op (path already present)
	if err := ensureGitignored(path); err != nil {
		t.Fatalf("second ensureGitignored() error: %v", err)
	}

	data, _ = os.ReadFile(".gitignore")
	if string(data) != path+"\n" {
		t.Errorf("after second call, .gitignore content = %q, want %q", string(data), path+"\n")
	}
}

func Test_ensureGitignored_existingContent(t *testing.T) {
	tmpDir := t.TempDir()
	chdir(t, tmpDir)

	// Create existing .gitignore without trailing newline
	if err := os.WriteFile(".gitignore", []byte("*.log"), 0o644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	path := "output/secret.md"
	if err := ensureGitignored(path); err != nil {
		t.Fatalf("ensureGitignored() error: %v", err)
	}

	data, _ := os.ReadFile(".gitignore")
	want := "*.log\n" + path + "\n"
	if string(data) != want {
		t.Errorf(".gitignore content = %q, want %q", string(data), want)
	}
}
