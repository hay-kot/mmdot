package commands

import (
	"os"
	"testing"
)

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

func Test_ensureGitignored_existingContentNoTrailingNewline(t *testing.T) {
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

func Test_ensureGitignored_existingContentWithTrailingNewline(t *testing.T) {
	tmpDir := t.TempDir()
	chdir(t, tmpDir)

	// Create existing .gitignore with trailing newline
	if err := os.WriteFile(".gitignore", []byte("*.log\n"), 0o644); err != nil {
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
