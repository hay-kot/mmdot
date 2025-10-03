package fcrypt

import (
	"fmt"
	"io"
	"os"

	"filippo.io/age"
	"filippo.io/age/armor"
)

// EncryptReader encrypts data from an io.Reader and writes the encrypted result to an io.Writer
func EncryptReader(r io.Reader, w io.Writer, recipient age.Recipient) error {
	armorWriter := armor.NewWriter(w)
	defer func() {
		_ = armorWriter.Close()
	}()

	// Create encryptor
	encryptor, err := age.Encrypt(armorWriter, recipient)
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

// EncryptFile encrypts a file in place removing the original version
func EncryptFile(inputPath, outputPath string, recipient age.Recipient) error {
	// Open input file
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer func() {
		_ = inputFile.Close()
	}()

	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() {
		_ = outputFile.Close()
	}()

	// Use EncryptReader to handle the encryption
	if err := EncryptReader(inputFile, outputFile, recipient); err != nil {
		return err
	}

	// Delete the original file
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

// DecryptFile decrypts a file leaving the original
func DecryptFile(inputPath, outputPath string, identity age.Identity) error {
	// Open input file
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer func() {
		_ = inputFile.Close()
	}()

	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() {
		_ = outputFile.Close()
	}()

	// Use DecryptReader to handle the decryption
	if err := DecryptReader(inputFile, outputFile, identity); err != nil {
		return err
	}

	return nil
}
