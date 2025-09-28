package core

import (
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func TestEncryptionIntegration(t *testing.T) {
	// Generate a test key pair
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("failed to generate identity: %v", err)
	}

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "encryption_integration_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Write identity to file
	identityPath := filepath.Join(tempDir, "identity.key")
	if err := os.WriteFile(identityPath, []byte(identity.String()), 0600); err != nil {
		t.Fatalf("failed to write identity file: %v", err)
	}

	// Create encryptor
	encryptor := NewEncryptor([]string{identity.Recipient().String()}, identityPath)

	// Test data - simulate a TOML config file
	testData := `[[hosts]]
name = "production-server"
hostname = "prod.example.com"
user = "deploy"
port = 22
identity_file = "~/.ssh/prod_key"

[[hosts]]
name = "staging-server"
hostname = "staging.example.com"
user = "deploy"
port = 2222`

	inputPath := filepath.Join(tempDir, "hosts.toml")
	encryptedPath := filepath.Join(tempDir, "hosts.toml.age")
	decryptedPath := filepath.Join(tempDir, "hosts_decrypted.toml")

	// Write test data
	if err := os.WriteFile(inputPath, []byte(testData), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	// Test file encryption/decryption
	t.Run("file encryption and decryption", func(t *testing.T) {
		// Encrypt file
		if err := encryptor.EncryptFile(inputPath, encryptedPath); err != nil {
			t.Fatalf("failed to encrypt file: %v", err)
		}

		// Verify encrypted file exists and is different from original
		encryptedData, err := os.ReadFile(encryptedPath)
		if err != nil {
			t.Fatalf("failed to read encrypted file: %v", err)
		}

		if string(encryptedData) == testData {
			t.Error("encrypted file should not match original data")
		}

		// Decrypt file
		if err := encryptor.DecryptFile(encryptedPath, decryptedPath); err != nil {
			t.Fatalf("failed to decrypt file: %v", err)
		}

		// Verify decrypted content matches original
		decryptedData, err := os.ReadFile(decryptedPath)
		if err != nil {
			t.Fatalf("failed to read decrypted file: %v", err)
		}

		if string(decryptedData) != testData {
			t.Errorf("decrypted data does not match original")
		}
	})

	// Test bytes encryption/decryption
	t.Run("bytes encryption and direct decryption", func(t *testing.T) {
		bytesEncryptedPath := filepath.Join(tempDir, "bytes.age")

		// Encrypt bytes directly
		if err := encryptor.EncryptBytes([]byte(testData), bytesEncryptedPath); err != nil {
			t.Fatalf("failed to encrypt bytes: %v", err)
		}

		// Decrypt to bytes
		decryptedBytes, err := encryptor.DecryptToBytes(bytesEncryptedPath)
		if err != nil {
			t.Fatalf("failed to decrypt to bytes: %v", err)
		}

		if string(decryptedBytes) != testData {
			t.Errorf("decrypted bytes do not match original")
		}
	})

	// Test error cases
	t.Run("error cases", func(t *testing.T) {
		// No recipients
		badEncryptor := NewEncryptor([]string{}, identityPath)
		if err := badEncryptor.EncryptFile(inputPath, "/tmp/bad.age"); err == nil {
			t.Error("expected error with no recipients")
		}

		// No identity
		badEncryptor2 := NewEncryptor([]string{identity.Recipient().String()}, "")
		if err := badEncryptor2.DecryptFile(encryptedPath, "/tmp/bad.txt"); err == nil {
			t.Error("expected error with no identity")
		}
	})
}