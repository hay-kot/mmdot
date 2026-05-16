package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/pkgs/fcrypt"
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

// ageFileNeedsEncrypt mirrors the logic in encrypt() for age.files.
// Returns true if the age file needs (re-)encryption.
func ageFileNeedsEncrypt(af core.AgeFile) (bool, error) {
	destInfo, err := os.Stat(af.Dest)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // no plaintext, nothing to encrypt
		}
		return false, err
	}

	srcInfo, err := os.Stat(af.Src)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil // encrypted file missing
		}
		return false, err
	}

	return destInfo.ModTime().After(srcInfo.ModTime()), nil
}

func testRecipients(t *testing.T) (*age.X25519Identity, []age.Recipient) {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	return id, []age.Recipient{id.Recipient()}
}

func TestAgeFileNeedsEncrypt_NoDest(t *testing.T) {
	tmpDir := t.TempDir()
	af := core.AgeFile{
		Src:  filepath.Join(tmpDir, "secret.age"),
		Dest: filepath.Join(tmpDir, "secret.json"),
	}

	needs, err := ageFileNeedsEncrypt(af)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if needs {
		t.Error("should not need encryption when plaintext dest doesn't exist")
	}
}

func TestAgeFileNeedsEncrypt_DestExistsSrcMissing(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "secret.json")
	if err := os.WriteFile(destPath, []byte("plaintext"), 0o600); err != nil {
		t.Fatal(err)
	}

	af := core.AgeFile{
		Src:  filepath.Join(tmpDir, "secret.age"),
		Dest: destPath,
	}

	needs, err := ageFileNeedsEncrypt(af)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !needs {
		t.Error("should need encryption when .age file is missing")
	}
}

func TestAgeFileNeedsEncrypt_BothExistSrcNewer(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "secret.json")
	srcPath := filepath.Join(tmpDir, "secret.age")

	// Write dest first
	if err := os.WriteFile(destPath, []byte("plaintext"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Set dest mtime to the past
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(destPath, past, past); err != nil {
		t.Fatal(err)
	}

	// Write src (encrypted) after — it's newer
	if err := os.WriteFile(srcPath, []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}

	af := core.AgeFile{Src: srcPath, Dest: destPath}

	needs, err := ageFileNeedsEncrypt(af)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if needs {
		t.Error("should NOT need encryption when .age file is newer than plaintext")
	}
}

func TestAgeFileNeedsEncrypt_BothExistDestNewer(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "secret.json")
	srcPath := filepath.Join(tmpDir, "secret.age")

	// Write src first
	if err := os.WriteFile(srcPath, []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Set src mtime to the past
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(srcPath, past, past); err != nil {
		t.Fatal(err)
	}

	// Write dest after — plaintext is newer
	if err := os.WriteFile(destPath, []byte("plaintext"), 0o600); err != nil {
		t.Fatal(err)
	}

	af := core.AgeFile{Src: srcPath, Dest: destPath}

	needs, err := ageFileNeedsEncrypt(af)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !needs {
		t.Error("should need re-encryption when plaintext is newer than .age file")
	}
}

func TestEncryptFileKeepSource(t *testing.T) {
	_, recipients := testRecipients(t)

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "secret.json")
	outputPath := filepath.Join(tmpDir, "secret.age")

	const plaintext = "sensitive data"
	if err := os.WriteFile(inputPath, []byte(plaintext), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := fcrypt.EncryptFileKeepSource(inputPath, outputPath, recipients); err != nil {
		t.Fatalf("EncryptFileKeepSource: %v", err)
	}

	// Plaintext source must still exist
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("plaintext source was deleted: %v", err)
	}
	if string(data) != plaintext {
		t.Errorf("plaintext content changed: got %q, want %q", data, plaintext)
	}

	// Encrypted output must exist
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("encrypted output missing: %v", err)
	}
}

func TestEncryptFileKeepSource_RoundTrip(t *testing.T) {
	id, recipients := testRecipients(t)

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "config.json")
	encPath := filepath.Join(tmpDir, "config.age")
	decPath := filepath.Join(tmpDir, "config-restored.json")

	const plaintext = `{"key": "value"}`
	if err := os.WriteFile(inputPath, []byte(plaintext), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := fcrypt.EncryptFileKeepSource(inputPath, encPath, recipients); err != nil {
		t.Fatalf("EncryptFileKeepSource: %v", err)
	}

	if err := fcrypt.DecryptFile(encPath, decPath, id); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}

	got, _ := os.ReadFile(decPath)
	if string(got) != plaintext {
		t.Errorf("round-trip failed: got %q, want %q", got, plaintext)
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
