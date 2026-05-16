package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/pkgs/fcrypt"
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

func TestAgeFileNeedsEncrypt_BothExistEqualMtime(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "secret.json")
	srcPath := filepath.Join(tmpDir, "secret.age")

	if err := os.WriteFile(srcPath, []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destPath, []byte("plaintext"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Set both to the same mtime
	now := time.Now()
	if err := os.Chtimes(srcPath, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(destPath, now, now); err != nil {
		t.Fatal(err)
	}

	af := core.AgeFile{Src: srcPath, Dest: destPath}

	needs, err := ageFileNeedsEncrypt(af)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if needs {
		t.Error("should NOT need encryption when mtimes are equal")
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

// runEncryptCmd builds a minimal CLI app and runs the encrypt subcommand with
// the provided arguments. It returns any error produced by the action.
func runEncryptCmd(t *testing.T, cfgPath string, args ...string) error {
	t.Helper()
	flags := &core.Flags{ConfigFilePath: cfgPath}
	app := &cli.Command{Name: "mmdot"}
	NewEncryptCmd(flags).Register(app)
	argv := append([]string{"mmdot", "encrypt"}, args...)
	return app.Run(context.Background(), argv)
}

// writeAgeConfig writes a minimal mmdot.yml that configures one age.files entry.
// recipientKey is the public age key string (e.g. "age1...").
func writeAgeConfig(t *testing.T, dir, recipientKey, src, dest string) string {
	t.Helper()
	cfgPath := filepath.Join(dir, "mmdot.yml")
	content := fmt.Sprintf(`version: 2
age:
  recipients:
    - %s
  files:
    - src: %s
      dest: %s
`, recipientKey, src, dest)
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writeAgeConfig: %v", err)
	}
	return cfgPath
}

// TestEncryptForce_AgeFile_ExistingNotOverwrittenWithoutForce checks that an
// existing .age file is preserved when --force is not set.
func TestEncryptForce_AgeFile_ExistingNotOverwrittenWithoutForce(t *testing.T) {
	id, recipients := testRecipients(t)

	tmpDir := t.TempDir()
	chdir(t, tmpDir)

	plainPath := filepath.Join(tmpDir, "secret.json")
	agePath := filepath.Join(tmpDir, "secret.json.age")

	// Write plaintext
	if err := os.WriteFile(plainPath, []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Create a stale .age file encrypted to a different recipient so the content
	// differs from what a fresh encryption would produce.
	staleID, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate stale identity: %v", err)
	}
	if err := fcrypt.EncryptFileKeepSource(plainPath, agePath, []age.Recipient{staleID.Recipient()}); err != nil {
		t.Fatalf("encrypt stale: %v", err)
	}

	// Set .age mtime newer than plaintext so the up-to-date check also skips it.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(agePath, future, future); err != nil {
		t.Fatal(err)
	}

	originalStat, err := os.Stat(agePath)
	if err != nil {
		t.Fatal(err)
	}

	cfgPath := writeAgeConfig(t, tmpDir, id.Recipient().String(), "secret.json.age", "secret.json")

	// Without --force the file should be skipped (up-to-date).
	if err := runEncryptCmd(t, cfgPath); err != nil {
		t.Fatalf("encrypt without --force returned error: %v", err)
	}

	afterStat, err := os.Stat(agePath)
	if err != nil {
		t.Fatal(err)
	}

	if !afterStat.ModTime().Equal(originalStat.ModTime()) {
		t.Error("--force absent: .age file was modified but should have been skipped")
	}

	// Confirm that decryption with the original identity still succeeds (file untouched).
	decPath := filepath.Join(tmpDir, "secret-check.json")
	if err := fcrypt.DecryptFile(agePath, decPath, staleID); err != nil {
		t.Errorf("decryption with original identity failed after no-force run: %v", err)
	}

	_ = recipients // satisfy compiler
}

// TestEncryptForce_AgeFile_ExistingOverwrittenWithForce checks that --force
// re-encrypts a file even when an up-to-date .age file exists.
func TestEncryptForce_AgeFile_ExistingOverwrittenWithForce(t *testing.T) {
	id, _ := testRecipients(t)

	tmpDir := t.TempDir()
	chdir(t, tmpDir)

	plainPath := filepath.Join(tmpDir, "secret.json")
	agePath := filepath.Join(tmpDir, "secret.json.age")

	// Write plaintext
	if err := os.WriteFile(plainPath, []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Create a stale .age file encrypted to a DIFFERENT identity.
	staleID, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate stale identity: %v", err)
	}
	if err := fcrypt.EncryptFileKeepSource(plainPath, agePath, []age.Recipient{staleID.Recipient()}); err != nil {
		t.Fatalf("encrypt stale: %v", err)
	}

	// Make .age newer than plaintext so the mtime check would normally skip it.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(agePath, future, future); err != nil {
		t.Fatal(err)
	}

	cfgPath := writeAgeConfig(t, tmpDir, id.Recipient().String(), "secret.json.age", "secret.json")

	// With --force the file must be re-encrypted.
	if err := runEncryptCmd(t, cfgPath, "--force"); err != nil {
		t.Fatalf("encrypt --force returned error: %v", err)
	}

	// The new .age file should be decryptable with id but NOT with staleID.
	decOK := filepath.Join(tmpDir, "secret-ok.json")
	if err := fcrypt.DecryptFile(agePath, decOK, id); err != nil {
		t.Errorf("decryption with new identity failed after --force: %v", err)
	}

	decFail := filepath.Join(tmpDir, "secret-fail.json")
	if err := fcrypt.DecryptFile(agePath, decFail, staleID); err == nil {
		t.Error("decryption with OLD identity succeeded after --force — file was not re-encrypted")
	}
}

// TestEncryptForce_VaultFile_ExistingOverwrittenWithForce checks that --force
// re-encrypts a vault variable file (variables.var_files with ?vault=true)
// even when the .age file already exists.
func TestEncryptForce_VaultFile_ExistingOverwrittenWithForce(t *testing.T) {
	id, _ := testRecipients(t)

	tmpDir := t.TempDir()
	chdir(t, tmpDir)

	plainPath := filepath.Join(tmpDir, "vault.yml")
	agePath := filepath.Join(tmpDir, "vault.yml.age")

	if err := os.WriteFile(plainPath, []byte("secret: value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Encrypt to a different (stale) identity first.
	staleID, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate stale identity: %v", err)
	}
	// Use EncryptFileKeepSource to preserve the plaintext — the vault encrypt path
	// expects the plaintext to exist alongside the .age file.
	if err := fcrypt.EncryptFileKeepSource(plainPath, agePath, []age.Recipient{staleID.Recipient()}); err != nil {
		t.Fatalf("encrypt stale vault: %v", err)
	}

	cfgContent := fmt.Sprintf(`version: 2
age:
  recipients:
    - %s
variables:
  var_files:
    - vault.yml.age?vault=true
`, id.Recipient().String())
	cfgPath := filepath.Join(tmpDir, "mmdot.yml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without --force: stale .age exists → should be skipped, stale identity still decryptable.
	if err := runEncryptCmd(t, cfgPath); err != nil {
		t.Fatalf("encrypt without --force returned error: %v", err)
	}
	decSkip := filepath.Join(tmpDir, "vault-skip.yml")
	if err := fcrypt.DecryptFile(agePath, decSkip, staleID); err != nil {
		t.Errorf("without --force stale identity should still decrypt: %v", err)
	}

	// With --force: must be re-encrypted to new recipients.
	if err := runEncryptCmd(t, cfgPath, "--force"); err != nil {
		t.Fatalf("encrypt --force returned error: %v", err)
	}

	decOK := filepath.Join(tmpDir, "vault-ok.yml")
	if err := fcrypt.DecryptFile(agePath, decOK, id); err != nil {
		t.Errorf("new identity should decrypt after --force: %v", err)
	}

	decFail := filepath.Join(tmpDir, "vault-fail.yml")
	if err := fcrypt.DecryptFile(agePath, decFail, staleID); err == nil {
		t.Error("stale identity still decrypts after --force — vault file was not re-encrypted")
	}
}
