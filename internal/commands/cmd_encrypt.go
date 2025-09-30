package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/pkgs/fcrypt"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

type EncryptCmd struct {
	coreFlags *core.Flags
}

func NewEncryptCmd(coreFlags *core.Flags) *EncryptCmd {
	return &EncryptCmd{coreFlags: coreFlags}
}

func (ec *EncryptCmd) Register(app *cli.Command) *cli.Command {
	cmds := []*cli.Command{
		{
			Name:  "encrypt",
			Usage: "encrypt all secrets files in-place",
			Description: `Encrypts all configured secret files using age encryption.

Files to encrypt are specified in mmdot.toml under various sections like:
- [ssh.secrets] for SSH private keys and configurations
- Template varsFile references

The command will:
- Use the configured age recipient (public key) for encryption
- Create .age encrypted versions of the files
- Skip files that are already encrypted
- Preserve original files after encryption

Encrypted files use the age format and can only be decrypted with the
corresponding age identity (private key).`,
			Action: ec.encrypt,
		},
		{
			Name:  "decrypt",
			Usage: "decrypt all secrets files in-place",
			Description: `Decrypts all configured .age encrypted files.

The command will:
- Use your configured age identity (private key) for decryption
- Restore the original unencrypted files
- Remove the .age encrypted versions after successful decryption
- Skip files that are already decrypted

This is typically used when you need to edit secret files or when setting up
a new machine from encrypted configuration files.`,
			Action: ec.decrypt,
		},
	}

	app.Commands = append(app.Commands, cmds...)
	return app
}

func (ec *EncryptCmd) encrypt(ctx context.Context, cmd *cli.Command) error {
	cfg, err := core.SetupEnv(ec.coreFlags.ConfigFilePath)
	if err != nil {
		return err
	}

	// Load the public key
	if len(cfg.Age.Recipients) == 0 {
		return fmt.Errorf("no age recipients configured in mmdot.toml")
	}

	recipient, err := fcrypt.LoadPublicKey(cfg.Age.Recipients[0])
	if err != nil {
		return fmt.Errorf("failed to load public key: %w", err)
	}

	files := cfg.EncryptedFiles()
	if len(files) == 0 {
		log.Info().Msg("No files configured for encryption")
		return nil
	}

	log.Info().Int("count", len(files)).Msg("Found files to encrypt")

	encryptedCount := 0
	for _, file := range files {
		var sourceFile, targetFile string

		// Determine source and target based on whether the configured file has .age extension
		if strings.HasSuffix(file, ".age") {
			// Config specifies encrypted file path
			sourceFile = strings.TrimSuffix(file, ".age")
			targetFile = file
		} else {
			// Config specifies plain file path
			sourceFile = file
			targetFile = file + ".age"
		}

		// Check if source file exists
		if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
			log.Debug().Str("file", sourceFile).Msg("Source file doesn't exist, skipping")
			continue
		}

		// Check if target file already exists
		if _, err := os.Stat(targetFile); err == nil {
			log.Debug().Str("file", targetFile).Msg("Encrypted file already exists, skipping")
			continue
		}

		// Encrypt the file
		log.Info().Str("source", sourceFile).Str("target", targetFile).Msg("Encrypting file")
		if err := fcrypt.EncryptFile(sourceFile, targetFile, recipient); err != nil {
			return fmt.Errorf("failed to encrypt %s: %w", sourceFile, err)
		}

		encryptedCount++
		log.Info().Str("file", targetFile).Msg("File encrypted successfully")
	}

	log.Info().Int("count", encryptedCount).Msg("Encryption complete")
	return nil
}

func (ec *EncryptCmd) decrypt(ctx context.Context, cmd *cli.Command) error {
	cfg, err := core.SetupEnv(ec.coreFlags.ConfigFilePath)
	if err != nil {
		return err
	}

	identity, err := cfg.Age.ReadIdentity()
	if err != nil {
		return err
	}

	files := cfg.EncryptedFiles()
	if len(files) == 0 {
		log.Info().Msg("No files configured for decryption")
		return nil
	}

	log.Info().Int("count", len(files)).Msg("Found files to decrypt")

	decryptedCount := 0
	for _, file := range files {
		var sourceFile, targetFile string

		// Determine source and target based on whether the configured file has .age extension
		if strings.HasSuffix(file, ".age") {
			// Config specifies encrypted file path
			sourceFile = file
			targetFile = strings.TrimSuffix(file, ".age")
		} else {
			// Config specifies plain file path - look for .age version
			sourceFile = file + ".age"
			targetFile = file
		}

		// Check if source file exists
		if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
			log.Debug().Str("file", sourceFile).Msg("Encrypted file doesn't exist, skipping")
			continue
		}

		// Check if target file already exists
		if _, err := os.Stat(targetFile); err == nil {
			log.Debug().Str("file", targetFile).Msg("Decrypted file already exists, skipping")
			continue
		}

		// Decrypt the file
		log.Info().Str("source", sourceFile).Str("target", targetFile).Msg("Decrypting file")
		if err := fcrypt.DecryptFile(sourceFile, targetFile, identity); err != nil {
			return fmt.Errorf("failed to decrypt %s: %w", sourceFile, err)
		}

		// Remove the encrypted file after successful decryption
		if err := os.Remove(sourceFile); err != nil {
			log.Warn().Str("file", sourceFile).Err(err).Msg("Failed to remove encrypted file after decryption")
		}

		decryptedCount++
		log.Info().Str("file", targetFile).Msg("File decrypted successfully")
	}

	log.Info().Int("count", decryptedCount).Msg("Decryption complete")
	return nil
}
