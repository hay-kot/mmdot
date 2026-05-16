package fcrypt

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"filippo.io/age/armor"
)

// CountStanzas returns the number of recipient stanzas in an armored .age file.
//
// This is used to detect recipient drift: if the stanza count differs from the
// number of configured recipients the file was encrypted with a different
// recipient set and should be re-encrypted.
//
// Heuristic: X25519 stanzas carry only the ephemeral wrapped key, not the
// recipient's public key, so exact recipient matching is impossible without the
// private identity. Stanza count is a practical proxy — one stanza is written
// per recipient — and lets us catch the common case of adding or removing
// recipients without requiring the identity to be present.
func CountStanzas(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	r := armor.NewReader(f)
	scanner := bufio.NewScanner(r)

	// First line must be the age version intro "age-encryption.org/v1".
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return 0, fmt.Errorf("read age intro for %s: %w", path, err)
		}
		return 0, fmt.Errorf("unexpected end of age header in %s", path)
	}

	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "-> "):
			// Recipient stanza header line.
			count++
		case strings.HasPrefix(line, "--- "):
			// Footer MAC line — header is complete.
			return count, nil
		}
		// Stanza body lines are base64 (chars [A-Za-z0-9+/]) and can never
		// start with "-> " or "--- ", so no false positives are possible.
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan age header for %s: %w", path, err)
	}

	// Reached EOF without the footer — treat as malformed but return what we
	// counted so callers can still make a best-effort decision.
	return count, nil
}
