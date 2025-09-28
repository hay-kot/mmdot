package core

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
)

// Encryptor handles age encryption and decryption operations
type Encryptor struct {
	// Recipients is a list of age public keys for encryption
	Recipients []string
	// Identity is the path to the age private key for decryption
	Identity string
}

// NewEncryptor creates a new Encryptor with the given recipients and identity
func NewEncryptor(recipients []string, identity string) *Encryptor {
	return &Encryptor{
		Recipients: recipients,
		Identity:   identity,
	}
}

// EncryptFile encrypts a file using age encryption with the configured recipients
func (e *Encryptor) EncryptFile(inputPath, outputPath string) error {
	if len(e.Recipients) == 0 {
		return errors.New("no recipients configured for encryption")
	}

	// Parse recipients
	recipients, err := e.parseRecipients()
	if err != nil {
		return fmt.Errorf("failed to parse recipients: %w", err)
	}

	// Read input file
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer func() { _ = inputFile.Close() }()

	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = outputFile.Close() }()

	// Create age writer
	w, err := age.Encrypt(outputFile, recipients...)
	if err != nil {
		return fmt.Errorf("failed to create age writer: %w", err)
	}

	// Copy input to encrypted output
	if _, err := io.Copy(w, inputFile); err != nil {
		return fmt.Errorf("failed to encrypt file: %w", err)
	}

	// Close the age writer to finalize encryption
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to finalize encryption: %w", err)
	}

	return nil
}

// DecryptFile decrypts an age-encrypted file using the configured identity
func (e *Encryptor) DecryptFile(inputPath, outputPath string) error {
	if e.Identity == "" {
		return errors.New("no identity configured for decryption")
	}

	// Load identity
	identity, err := e.loadIdentity()
	if err != nil {
		return fmt.Errorf("failed to load identity: %w", err)
	}

	// Open encrypted file
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open encrypted file: %w", err)
	}
	defer func() { _ = inputFile.Close() }()

	// Create age reader
	r, err := age.Decrypt(inputFile, identity)
	if err != nil {
		return fmt.Errorf("failed to decrypt file: %w", err)
	}

	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = outputFile.Close() }()

	// Copy decrypted content to output
	if _, err := io.Copy(outputFile, r); err != nil {
		return fmt.Errorf("failed to write decrypted content: %w", err)
	}

	return nil
}

// DecryptToBytes decrypts an age-encrypted file and returns the content as bytes
func (e *Encryptor) DecryptToBytes(inputPath string) ([]byte, error) {
	if e.Identity == "" {
		return nil, errors.New("no identity configured for decryption")
	}

	// Load identity
	identity, err := e.loadIdentity()
	if err != nil {
		return nil, fmt.Errorf("failed to load identity: %w", err)
	}

	// Open encrypted file
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open encrypted file: %w", err)
	}
	defer func() { _ = inputFile.Close() }()

	// Create age reader
	r, err := age.Decrypt(inputFile, identity)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt file: %w", err)
	}

	// Read all decrypted content
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read decrypted content: %w", err)
	}

	return content, nil
}

// EncryptBytes encrypts bytes using age encryption with the configured recipients
func (e *Encryptor) EncryptBytes(data []byte, outputPath string) error {
	if len(e.Recipients) == 0 {
		return errors.New("no recipients configured for encryption")
	}

	// Parse recipients
	recipients, err := e.parseRecipients()
	if err != nil {
		return fmt.Errorf("failed to parse recipients: %w", err)
	}

	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = outputFile.Close() }()

	// Create age writer
	w, err := age.Encrypt(outputFile, recipients...)
	if err != nil {
		return fmt.Errorf("failed to create age writer: %w", err)
	}

	// Write data to encrypted output
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to encrypt data: %w", err)
	}

	// Close the age writer to finalize encryption
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to finalize encryption: %w", err)
	}

	return nil
}

// parseRecipients parses the recipient strings into age recipients
func (e *Encryptor) parseRecipients() ([]age.Recipient, error) {
	recipients := make([]age.Recipient, 0, len(e.Recipients))

	for _, recipientStr := range e.Recipients {
		recipientStr = strings.TrimSpace(recipientStr)
		if recipientStr == "" {
			continue
		}

		// Parse as age public key
		if strings.HasPrefix(recipientStr, "age1") {
			recipient, err := age.ParseX25519Recipient(recipientStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse age recipient %q: %w", recipientStr, err)
			}
			recipients = append(recipients, recipient)
		} else {
			return nil, fmt.Errorf("unsupported recipient format: %q (must be age public key starting with 'age1')", recipientStr)
		}
	}

	if len(recipients) == 0 {
		return nil, errors.New("no valid recipients found")
	}

	return recipients, nil
}

// loadIdentity loads the identity from the configured identity path
func (e *Encryptor) loadIdentity() (age.Identity, error) {
	identityFile, err := os.Open(e.Identity)
	if err != nil {
		return nil, fmt.Errorf("failed to open identity file: %w", err)
	}
	defer func() { _ = identityFile.Close() }()

	// Parse as age identity
	identities, err := age.ParseIdentities(identityFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse identity file as age key: %w", err)
	}

	if len(identities) == 0 {
		return nil, fmt.Errorf("no identities found in identity file")
	}

	return identities[0], nil
}

// LoadIdentitiesFromFile loads multiple identities from a file
func LoadIdentitiesFromFile(path string) ([]age.Identity, error) {
	identityFile, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open identity file: %w", err)
	}
	defer func() { _ = identityFile.Close() }()

	// Parse as age identities
	identities, err := age.ParseIdentities(identityFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse identities from %s: %w", path, err)
	}

	if len(identities) == 0 {
		return nil, fmt.Errorf("no valid identities found in %s", path)
	}

	return identities, nil
}

// ParseRecipientsFromFile reads recipients from a file (one per line)
func ParseRecipientsFromFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open recipients file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var recipients []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			recipients = append(recipients, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read recipients file: %w", err)
	}

	return recipients, nil
}