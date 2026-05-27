package fcrypt

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"filippo.io/age/armor"
)

// CountRecipientStanzas returns the number of recipient stanzas ("-> ...") in
// the header of an armored age file. This is a cheap proxy for "how many keys
// can decrypt this file" and is used to detect when the configured recipient
// list has grown or shrunk since the file was last encrypted.
func CountRecipientStanzas(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	r := armor.NewReader(f)
	br := bufio.NewReader(r)

	count := 0
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return 0, fmt.Errorf("unexpected EOF parsing age header in %s", path)
			}
			return 0, fmt.Errorf("read age header in %s: %w", path, err)
		}
		line = strings.TrimRight(line, "\n")

		switch {
		case strings.HasPrefix(line, "-> "):
			count++
		case strings.HasPrefix(line, "--- "):
			return count, nil
		}
	}
}

// RecryptFile decrypts an age file into memory and re-encrypts it back to the
// same path using the supplied recipients. The file is replaced atomically via
// a temp file + rename.
func RecryptFile(path string, identity age.Identity, recipients []age.Recipient) (err error) {
	in, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = in.Close() }()

	var plaintext bytes.Buffer
	if err := DecryptReader(in, &plaintext, identity); err != nil {
		return fmt.Errorf("decrypt %s: %w", path, err)
	}
	_ = in.Close()

	tmpFile, err := os.CreateTemp(filepath.Dir(path), ".mmdot-rotate-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmpFile.Name())
		}
	}()

	if err = EncryptReader(&plaintext, tmpFile, recipients); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("re-encrypt %s: %w", path, err)
	}

	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err = os.Rename(tmpFile.Name(), path); err != nil {
		return fmt.Errorf("rename temp file to %s: %w", path, err)
	}
	return nil
}
