package fcrypt

import (
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func newIdentities(t *testing.T, n int) ([]*age.X25519Identity, []age.Recipient) {
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

func TestCountRecipientStanzas(t *testing.T) {
	cases := []int{1, 2, 5}
	for _, n := range cases {
		_, recs := newIdentities(t, n)
		tmpDir := t.TempDir()
		in := filepath.Join(tmpDir, "p.txt")
		out := filepath.Join(tmpDir, "p.age")
		if err := os.WriteFile(in, []byte("hi"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := EncryptFileKeepSource(in, out, recs); err != nil {
			t.Fatal(err)
		}
		got, err := CountRecipientStanzas(out)
		if err != nil {
			t.Fatalf("CountRecipientStanzas: %v", err)
		}
		if got != n {
			t.Errorf("count = %d, want %d", got, n)
		}
	}
}

func TestRecryptFile_AddsNewRecipient(t *testing.T) {
	ids, recs := newIdentities(t, 1)
	tmpDir := t.TempDir()
	in := filepath.Join(tmpDir, "p.txt")
	enc := filepath.Join(tmpDir, "p.age")

	const plaintext = "rotation works"
	if err := os.WriteFile(in, []byte(plaintext), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EncryptFileKeepSource(in, enc, recs); err != nil {
		t.Fatal(err)
	}

	// Add a second recipient and rotate using the original identity.
	newIDs, _ := newIdentities(t, 1)
	allRecs := []age.Recipient{recs[0], newIDs[0].Recipient()}
	if err := RecryptFile(enc, ids[0], allRecs); err != nil {
		t.Fatalf("RecryptFile: %v", err)
	}

	count, err := CountRecipientStanzas(enc)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("stanza count after rotation = %d, want 2", count)
	}

	// New identity must now be able to decrypt.
	out := filepath.Join(tmpDir, "out.txt")
	if err := DecryptFile(enc, out, newIDs[0]); err != nil {
		t.Fatalf("decrypt with new identity: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != plaintext {
		t.Errorf("decrypted = %q, want %q", got, plaintext)
	}
}
