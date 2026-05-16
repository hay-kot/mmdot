package fcrypt

import (
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func generateIdentities(t *testing.T, n int) ([]*age.X25519Identity, []age.Recipient) {
	t.Helper()
	ids := make([]*age.X25519Identity, n)
	recs := make([]age.Recipient, n)
	for i := range n {
		id, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatalf("generate identity %d: %v", i, err)
		}
		ids[i] = id
		recs[i] = id.Recipient()
	}
	return ids, recs
}

func TestCountStanzas_SingleRecipient(t *testing.T) {
	_, recs := generateIdentities(t, 1)

	dir := t.TempDir()
	plainPath := filepath.Join(dir, "secret.txt")
	encPath := filepath.Join(dir, "secret.txt.age")

	if err := os.WriteFile(plainPath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EncryptFile(plainPath, encPath, recs); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	got, err := CountStanzas(encPath)
	if err != nil {
		t.Fatalf("CountStanzas: %v", err)
	}
	if got != 1 {
		t.Errorf("CountStanzas = %d, want 1", got)
	}
}

func TestCountStanzas_MultipleRecipients(t *testing.T) {
	const n = 3
	_, recs := generateIdentities(t, n)

	dir := t.TempDir()
	plainPath := filepath.Join(dir, "secret.txt")
	encPath := filepath.Join(dir, "secret.txt.age")

	if err := os.WriteFile(plainPath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EncryptFile(plainPath, encPath, recs); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	got, err := CountStanzas(encPath)
	if err != nil {
		t.Fatalf("CountStanzas: %v", err)
	}
	if got != n {
		t.Errorf("CountStanzas = %d, want %d", got, n)
	}
}

func TestCountStanzas_FileNotFound(t *testing.T) {
	_, err := CountStanzas("/nonexistent/path/file.age")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestCountStanzas_TwoRecipients_DetectsDrift(t *testing.T) {
	// Simulate drift: encrypt to 1 recipient, configured count is 2.
	_, recs1 := generateIdentities(t, 1)

	dir := t.TempDir()
	plainPath := filepath.Join(dir, "vault.yml")
	encPath := filepath.Join(dir, "vault.yml.age")

	if err := os.WriteFile(plainPath, []byte("key: value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EncryptFile(plainPath, encPath, recs1); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	got, err := CountStanzas(encPath)
	if err != nil {
		t.Fatalf("CountStanzas: %v", err)
	}

	configuredRecipients := 2
	if got == configuredRecipients {
		t.Errorf("drift not detected: CountStanzas = %d, configured = %d (should differ)", got, configuredRecipients)
	}
	if got != 1 {
		t.Errorf("CountStanzas = %d, want 1", got)
	}
}
