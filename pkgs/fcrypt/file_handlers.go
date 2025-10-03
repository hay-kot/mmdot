package fcrypt

import (
	"fmt"
	"os"
	"strings"

	"filippo.io/age"
)

// EncryptInPlace takes a filepath <name>.<ext>. This file is then replaced with an
// encrypted version with a new file <name>.<ext>.age. The old file is removed
// without confirmation and cannot be recovered.
func EncryptInPlace(filepath string, pubkey age.Recipient) error {
	outputPath := filepath + ".age"
	return EncryptFile(filepath, outputPath, pubkey)
}

// DecryptInPlace takes in a file that is assumed to be encrypted and replaces that file
// with a version that is decrypted in-place. The file left in place is created as
// <name>.<ext> with the suffix .age now removed from the filename
func DecryptInPlace(filepath string, privatekey age.Identity) error {
	if !strings.HasSuffix(filepath, ".age") {
		return fmt.Errorf("file %s does not have .age extension", filepath)
	}
	outputPath := strings.TrimSuffix(filepath, ".age")
	if err := DecryptFile(filepath, outputPath, privatekey); err != nil {
		return err
	}
	// Remove the encrypted file after successful decryption
	return os.Remove(filepath)
}
