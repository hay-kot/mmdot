package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/internal/ssh"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

type SSHCmd struct {
	flags *core.Flags
}

func NewSSHCmd(flags *core.Flags) *SSHCmd {
	return &SSHCmd{flags: flags}
}

func (sc *SSHCmd) Register(app *cli.Command) *cli.Command {
	cmd := &cli.Command{
		Name:  "ssh",
		Usage: "Manage SSH configurations with encryption support",
		Commands: []*cli.Command{
			{
				Name:  "encrypt",
				Usage: "Encrypt all SSH host files referenced in configuration",
				Description: `Finds all SSH host sources in mmdot.toml that have unencrypted files
and encrypts them using available age keys.`,
				Action: sc.encrypt,
			},
			{
				Name:  "decrypt",
				Usage: "Decrypt all SSH host files we have keys for",
				Description: `Finds all encrypted SSH host sources in mmdot.toml and attempts to decrypt them
using available age identity keys.`,
				Action: sc.decrypt,
			},
		},
	}

	app.Commands = append(app.Commands, cmd)
	return app
}

func (sc *SSHCmd) encrypt(ctx context.Context, c *cli.Command) error {
	cfg, err := setupEnv(sc.flags.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if len(cfg.SSH.Hosts) == 0 {
		fmt.Println("No SSH host sources configured in mmdot.toml")
		return nil
	}

	// Find available age keys from configuration
	recipients, err := sc.findAgeRecipientsFromConfig(cfg.SSH.Hosts)
	if err != nil {
		return fmt.Errorf("failed to find age recipients: %w", err)
	}

	if len(recipients) == 0 {
		return fmt.Errorf("no age recipients found in configuration")
	}

	fmt.Printf("Found %d age recipients\n", len(recipients))

	encryptor := core.NewEncryptor(recipients, "")
	var encrypted int

	// Process each host source
	for _, source := range cfg.SSH.Hosts {
		// Skip if it has inline hosts (nothing to encrypt)
		if len(source.Hosts) > 0 {
			fmt.Printf("⚠ Source %s uses inline hosts (no file to encrypt)\n", source.Name)
			continue
		}

		// Skip if no encryption configured (no recipients)
		if len(source.Recipients) == 0 {
			fmt.Printf("⚠ Source %s has no recipients configured for encryption\n", source.Name)
			continue
		}

		// Skip if encrypted file already exists and source file doesn't
		if source.EncryptedFile != "" {
			if _, err := os.Stat(source.EncryptedFile); err == nil {
				// Encrypted file exists, check if source file also exists
				sourceFile := strings.TrimSuffix(source.EncryptedFile, ".age")
				if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
					fmt.Printf("⚠ Source %s already encrypted (no source file found)\n", source.Name)
					continue
				}
			}
		}

		// For sources that should have files, we need to determine the file path
		// Use the encrypted file path minus the .age extension as the source
		tomlFile := source.EncryptedFile
		if strings.HasSuffix(tomlFile, ".age") {
			tomlFile = strings.TrimSuffix(tomlFile, ".age")
		} else {
			// Fallback to convention like <source-name>.toml
			tomlFile = fmt.Sprintf("%s.toml", source.Name)
		}

		// Check if the file exists
		if _, err := os.Stat(tomlFile); os.IsNotExist(err) {
			log.Debug().Str("source", source.Name).Str("file", tomlFile).Msg("no file found for source")
			continue
		}

		// Validate it's a proper hosts file
		var hostsFile ssh.HostsFile
		if _, err := toml.DecodeFile(tomlFile, &hostsFile); err != nil {
			log.Warn().Str("file", tomlFile).Err(err).Msg("skipping invalid TOML file")
			continue
		}

		if len(hostsFile.Hosts) == 0 {
			log.Debug().Str("file", tomlFile).Msg("skipping file with no hosts")
			continue
		}

		// Validate hosts
		if err := ssh.ValidateHosts(hostsFile.Hosts); err != nil {
			log.Warn().Str("file", tomlFile).Err(err).Msg("skipping file with invalid hosts")
			continue
		}

		outputFile := tomlFile + ".age"

		log.Info().
			Str("source", source.Name).
			Str("input", tomlFile).
			Str("output", outputFile).
			Int("hosts", len(hostsFile.Hosts)).
			Msg("encrypting hosts file")

		if err := encryptor.EncryptFile(tomlFile, outputFile); err != nil {
			log.Error().Str("file", tomlFile).Err(err).Msg("failed to encrypt file")
			continue
		}

		// Delete the source file after successful encryption
		if err := os.Remove(tomlFile); err != nil {
			log.Error().Str("file", tomlFile).Err(err).Msg("failed to remove source file after encryption")
			continue
		}

		fmt.Printf("✓ Encrypted %s -> %s (%d hosts)\n", tomlFile, outputFile, len(hostsFile.Hosts))
		encrypted++
	}

	if encrypted == 0 {
		fmt.Println("No files found to encrypt")
	} else {
		fmt.Printf("\nSuccessfully encrypted %d files\n", encrypted)
	}

	return nil
}

func (sc *SSHCmd) decrypt(ctx context.Context, c *cli.Command) error {
	cfg, err := setupEnv(sc.flags.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if len(cfg.SSH.Hosts) == 0 {
		fmt.Println("No SSH host sources configured in mmdot.toml")
		return nil
	}

	fmt.Printf("Processing encrypted SSH host sources\n")
	var decrypted int

	// Process each encrypted host source
	for _, source := range cfg.SSH.Hosts {
		// Skip if not encrypted
		if !source.NeedsDecryption() {
			log.Debug().Str("source", source.Name).Msg("source is not encrypted")
			continue
		}

		// Check if encrypted file exists
		if _, err := os.Stat(source.EncryptedFile); os.IsNotExist(err) {
			log.Warn().Str("source", source.Name).Str("file", source.EncryptedFile).Msg("encrypted file not found")
			continue
		}

		// Determine output file name (remove .age extension if present)
		outputFile := source.EncryptedFile
		if strings.HasSuffix(outputFile, ".age") {
			outputFile = strings.TrimSuffix(outputFile, ".age")
		} else {
			outputFile = outputFile + ".decrypted"
		}

		log.Info().
			Str("source", source.Name).
			Str("input", source.EncryptedFile).
			Str("output", outputFile).
			Msg("decrypting file")

		// Use source-specific identity file
		if source.IdentityFile == "" {
			log.Warn().Str("source", source.Name).Msg("no identity file specified for encrypted source")
			continue
		}

		sourceEncryptor := core.NewEncryptor(nil, source.IdentityFile)

		// Try to decrypt
		if err := sourceEncryptor.DecryptFile(source.EncryptedFile, outputFile); err != nil {
			log.Warn().Str("source", source.Name).Str("file", source.EncryptedFile).Err(err).Msg("failed to decrypt file")
			continue
		}

		// Validate it's a valid hosts file
		var hostsFile ssh.HostsFile
		if _, err := toml.DecodeFile(outputFile, &hostsFile); err != nil {
			log.Warn().Str("file", outputFile).Err(err).Msg("decrypted file is not a valid hosts file")
			continue
		}

		// Delete the encrypted file after successful decryption
		if err := os.Remove(source.EncryptedFile); err != nil {
			log.Error().Str("file", source.EncryptedFile).Err(err).Msg("failed to remove encrypted file after decryption")
			continue
		}

		fmt.Printf("✓ Decrypted %s -> %s (%d hosts)\n", source.EncryptedFile, outputFile, len(hostsFile.Hosts))
		decrypted++
	}

	if decrypted == 0 {
		fmt.Println("No encrypted files could be decrypted")
	} else {
		fmt.Printf("\nSuccessfully decrypted %d files\n", decrypted)
	}

	return nil
}

// findAgeRecipientsFromConfig finds age recipients from SSH host sources configuration
func (sc *SSHCmd) findAgeRecipientsFromConfig(sources []ssh.HostSource) ([]string, error) {
	var allRecipients []string
	recipientSet := make(map[string]bool)

	for _, source := range sources {
		// Collect recipients from each source
		for _, recipient := range source.Recipients {
			if recipient != "" && !recipientSet[recipient] {
				allRecipients = append(allRecipients, recipient)
				recipientSet[recipient] = true
			}
		}
	}

	return allRecipients, nil
}
