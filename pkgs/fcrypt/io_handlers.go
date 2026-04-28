package fcrypt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"filippo.io/age"
	"filippo.io/age/armor"
)

// EncryptReader encrypts data from an io.Reader and writes the encrypted result to an io.Writer
func EncryptReader(r io.Reader, w io.Writer, recipients []age.Recipient) error {
	armorWriter := armor.NewWriter(w)
	defer func() {
		_ = armorWriter.Close()
	}()

	// Create encryptor
	encryptor, err := age.Encrypt(armorWriter, recipients...)
	if err != nil {
		return fmt.Errorf("failed to create encryptor: %w", err)
	}
	defer func() {
		_ = encryptor.Close()
	}()

	// Copy data from input to encryptor
	_, err = io.Copy(encryptor, r)
	if err != nil {
		return fmt.Errorf("failed to encrypt: %w", err)
	}

	// Explicitly close in reverse order to ensure proper finalization
	if err = encryptor.Close(); err != nil {
		_ = armorWriter.Close()
		return fmt.Errorf("failed to finalize encryption: %w", err)
	}
	if err = armorWriter.Close(); err != nil {
		return fmt.Errorf("failed to finalize armor: %w", err)
	}

	return nil
}

// EncryptFile encrypts a file in place removing the original version.
// It writes to a temporary file first and renames on success to avoid
// leaving a partially-written output file on failure.
func EncryptFile(inputPath, outputPath string, recipients []age.Recipient) (err error) {
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer func() {
		_ = inputFile.Close()
	}()

	tmpFile, err := os.CreateTemp(filepath.Dir(outputPath), ".mmdot-encrypt-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmpFile.Name())
		}
	}()

	if err = EncryptReader(inputFile, tmpFile, recipients); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err = os.Rename(tmpFile.Name(), outputPath); err != nil {
		return fmt.Errorf("failed to rename temp file to output: %w", err)
	}

	if err = os.Remove(inputPath); err != nil {
		return err
	}

	return nil
}

// DecryptReader decrypts data from an io.Reader and writes the decrypted result to an io.Writer
func DecryptReader(r io.Reader, w io.Writer, identity age.Identity) error {
	// Create armor reader
	armorReader := armor.NewReader(r)

	// Create decryptor
	decryptor, err := age.Decrypt(armorReader, identity)
	if err != nil {
		return fmt.Errorf("failed to create decryptor: %w", err)
	}

	// Copy data from decryptor to output
	_, err = io.Copy(w, decryptor)
	if err != nil {
		return fmt.Errorf("failed to decrypt: %w", err)
	}

	return nil
}

// DecryptFile decrypts a file leaving the original.
// It writes to a temporary file first and renames on success to avoid
// leaving a partially-written output file on failure.
func DecryptFile(inputPath, outputPath string, identity age.Identity) (err error) {
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer func() {
		_ = inputFile.Close()
	}()

	tmpFile, err := os.CreateTemp(filepath.Dir(outputPath), ".mmdot-decrypt-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmpFile.Name())
		}
	}()

	if err = DecryptReader(inputFile, tmpFile, identity); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err = os.Rename(tmpFile.Name(), outputPath); err != nil {
		return fmt.Errorf("failed to rename temp file to output: %w", err)
	}

	return nil
}
