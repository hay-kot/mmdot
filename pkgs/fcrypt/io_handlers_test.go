package fcrypt

import (
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func TestEncryptDecryptRoundtrip_MultipleRecipients(t *testing.T) {
	const numRecipients = 3
	const plaintext = "the quick brown fox jumps over the lazy dog"

	identities := make([]*age.X25519Identity, numRecipients)
	recipients := make([]age.Recipient, numRecipients)
	for i := range numRecipients {
		id, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatalf("generate identity %d: %v", i, err)
		}
		identities[i] = id
		recipients[i] = id.Recipient()
	}

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "plain.txt")
	encryptedPath := filepath.Join(tmpDir, "plain.txt.age")

	if err := os.WriteFile(inputPath, []byte(plaintext), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	// Encrypt with all recipients.
	if err := EncryptFile(inputPath, encryptedPath, recipients); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	// EncryptFile removes the input; verify it's gone.
	if _, err := os.Stat(inputPath); !os.IsNotExist(err) {
		t.Fatal("expected input file to be removed after encryption")
	}

	// Decrypt with each identity individually.
	for i, id := range identities {
		outPath := filepath.Join(tmpDir, "decrypted"+string(rune('0'+i))+".txt")
		if err := DecryptFile(encryptedPath, outPath, id); err != nil {
			t.Fatalf("DecryptFile with identity %d: %v", i, err)
		}

		got, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("read decrypted output %d: %v", i, err)
		}
		if string(got) != plaintext {
			t.Errorf("identity %d: got %q, want %q", i, got, plaintext)
		}
	}
}

func TestEncryptFile_NoOutputOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "does-not-exist.txt")
	outputPath := filepath.Join(tmpDir, "output.age")

	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	err = EncryptFile(nonExistent, outputPath, []age.Recipient{id.Recipient()})
	if err == nil {
		t.Fatal("expected error for non-existent input")
	}

	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatal("output file should not exist after encryption failure")
	}

	// Also verify no temp files leaked into the directory.
	entries, _ := filepath.Glob(filepath.Join(tmpDir, ".mmdot-encrypt-*"))
	if len(entries) > 0 {
		t.Fatalf("temp files leaked: %v", entries)
	}
}

func TestDecryptFile_NoOutputOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	badInput := filepath.Join(tmpDir, "not-encrypted.txt")
	outputPath := filepath.Join(tmpDir, "decrypted.txt")

	if err := os.WriteFile(badInput, []byte("this is not age-encrypted data"), 0o600); err != nil {
		t.Fatalf("write bad input: %v", err)
	}

	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	err = DecryptFile(badInput, outputPath, id)
	if err == nil {
		t.Fatal("expected error for non-encrypted input")
	}

	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatal("output file should not exist after decryption failure")
	}

	// Also verify no temp files leaked into the directory.
	entries, _ := filepath.Glob(filepath.Join(tmpDir, ".mmdot-decrypt-*"))
	if len(entries) > 0 {
		t.Fatalf("temp files leaked: %v", entries)
	}
}

func TestLoadPublicKeys(t *testing.T) {
	const numKeys = 3

	keyStrings := make([]string, numKeys)
	for i := range numKeys {
		id, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatalf("generate identity %d: %v", i, err)
		}
		keyStrings[i] = id.Recipient().String()
	}

	// Valid keys should all load.
	got, err := LoadPublicKeys(keyStrings)
	if err != nil {
		t.Fatalf("LoadPublicKeys with valid keys: %v", err)
	}
	if len(got) != numKeys {
		t.Fatalf("got %d recipients, want %d", len(got), numKeys)
	}

	// One invalid key mixed in should produce an error.
	badKeys := append([]string{}, keyStrings...)
	badKeys[1] = "not-a-valid-age-key"
	_, err = LoadPublicKeys(badKeys)
	if err == nil {
		t.Fatal("expected error for invalid key in slice")
	}
}
