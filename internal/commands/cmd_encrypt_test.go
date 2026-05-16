package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hay-kot/mmdot/internal/core"
	"github.com/urfave/cli/v3"
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

// writeMinimalConfig writes a minimal mmdot.yml with the given age.files entries.
func writeMinimalConfig(t *testing.T, dir string, ageFiles []core.AgeFile) {
	t.Helper()

	cfg := "version: 2\n"
	if len(ageFiles) > 0 {
		cfg += "age:\n  files:\n"
		for _, af := range ageFiles {
			cfg += "    - src: " + af.Src + "\n"
			cfg += "      dest: " + af.Dest + "\n"
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "mmdot.yml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
}

// writeFile creates a file with content and optional modtime.
func writeFile(t *testing.T, path string, content string, modTime ...time.Time) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create dirs for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
	if len(modTime) > 0 {
		if err := os.Chtimes(path, modTime[0], modTime[0]); err != nil {
			t.Fatalf("failed to set modtime on %s: %v", path, err)
		}
	}
}

// runEncryptDryRun sets up an EncryptCmd with --dry-run and invokes encrypt.
func runEncryptDryRun(t *testing.T, configPath string) error {
	t.Helper()

	flags := &core.Flags{ConfigFilePath: configPath}
	ec := &EncryptCmd{coreFlags: flags, dryRun: true}

	return ec.encrypt(context.Background(), &cli.Command{})
}

func TestEncryptDryRun_AgeFiles_BothExist_SrcNewer(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now()
	srcPath := filepath.Join(tmpDir, "secrets", "file.age")
	destPath := filepath.Join(tmpDir, "output", "file.txt")

	writeMinimalConfig(t, tmpDir, []core.AgeFile{
		{Src: srcPath, Dest: destPath},
	})

	// src (.age) is newer than dest (plaintext) — normal post-decrypt state
	writeFile(t, destPath, "plaintext", now.Add(-time.Minute))
	writeFile(t, srcPath, "encrypted", now)

	err := runEncryptDryRun(t, filepath.Join(tmpDir, "mmdot.yml"))
	if err != nil {
		t.Errorf("expected no error when .age src is newer, got: %v", err)
	}
}

func TestEncryptDryRun_AgeFiles_BothExist_DestNewer(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now()
	srcPath := filepath.Join(tmpDir, "secrets", "file.age")
	destPath := filepath.Join(tmpDir, "output", "file.txt")

	writeMinimalConfig(t, tmpDir, []core.AgeFile{
		{Src: srcPath, Dest: destPath},
	})

	// dest (plaintext) is newer than src (.age) — user edited the plaintext
	writeFile(t, srcPath, "encrypted", now.Add(-time.Minute))
	writeFile(t, destPath, "modified plaintext", now)

	err := runEncryptDryRun(t, filepath.Join(tmpDir, "mmdot.yml"))
	if err == nil {
		t.Error("expected error when plaintext is newer than .age, got nil")
	}
}

func TestEncryptDryRun_AgeFiles_OnlyDestExists(t *testing.T) {
	tmpDir := t.TempDir()

	srcPath := filepath.Join(tmpDir, "secrets", "file.age")
	destPath := filepath.Join(tmpDir, "output", "file.txt")

	writeMinimalConfig(t, tmpDir, []core.AgeFile{
		{Src: srcPath, Dest: destPath},
	})

	// Only plaintext exists, no .age file — genuinely unencrypted
	writeFile(t, destPath, "plaintext")

	err := runEncryptDryRun(t, filepath.Join(tmpDir, "mmdot.yml"))
	if err == nil {
		t.Error("expected error when .age src doesn't exist, got nil")
	}
}

func TestEncryptDryRun_AgeFiles_OnlySrcExists(t *testing.T) {
	tmpDir := t.TempDir()

	srcPath := filepath.Join(tmpDir, "secrets", "file.age")
	destPath := filepath.Join(tmpDir, "output", "file.txt")

	writeMinimalConfig(t, tmpDir, []core.AgeFile{
		{Src: srcPath, Dest: destPath},
	})

	// Only .age exists, no plaintext — fully encrypted state
	writeFile(t, srcPath, "encrypted")

	err := runEncryptDryRun(t, filepath.Join(tmpDir, "mmdot.yml"))
	if err != nil {
		t.Errorf("expected no error when only .age src exists, got: %v", err)
	}
}

func TestEncryptDryRun_AgeFiles_NeitherExists(t *testing.T) {
	tmpDir := t.TempDir()

	srcPath := filepath.Join(tmpDir, "secrets", "file.age")
	destPath := filepath.Join(tmpDir, "output", "file.txt")

	writeMinimalConfig(t, tmpDir, []core.AgeFile{
		{Src: srcPath, Dest: destPath},
	})

	// Neither file exists — nothing to do
	err := runEncryptDryRun(t, filepath.Join(tmpDir, "mmdot.yml"))
	if err != nil {
		t.Errorf("expected no error when neither file exists, got: %v", err)
	}
}
