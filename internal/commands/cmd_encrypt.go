package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/pkgs/fcrypt"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

type EncryptCmd struct {
	coreFlags *core.Flags
	dryRun    bool
	force     bool
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

Files to encrypt are specified in mmdot.yaml under various sections like:
- [ssh.secrets] for SSH private keys and configurations
- Template varsFile references

The command will:
- Use the configured age recipient (public key) for encryption
- Create .age encrypted versions of the files
- Skip files that are already encrypted
- Preserve original files after encryption

Encrypted files use the age format and can only be decrypted with the
corresponding age identity (private key).`,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:        "dry-run",
					Usage:       "check if files need encryption without encrypting them",
					Destination: &ec.dryRun,
				},
				&cli.BoolFlag{
					Name:        "force",
					Aliases:     []string{"f"},
					Usage:       "re-encrypt all files regardless of mtime or recipient count",
					Destination: &ec.force,
				},
			},
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

	// Collect vault files that need encryption.
	//
	// A vault file needs (re-)encryption when:
	//   1. The .age file is missing (new file).
	//   2. --force is set (unconditional re-encryption).
	//   3. Recipient drift: the stanza count in the .age file differs from the
	//      configured recipient count. Age X25519 stanzas carry only the
	//      ephemeral wrapped key, not the recipient public key, so exact
	//      matching is impossible without an identity. Stanza count is a
	//      reliable proxy: age writes exactly one stanza per recipient.
	vaultFilesToEncrypt := []string{}
	for _, file := range cfg.EncryptedFiles() {
		var sourceFile, targetFile string

		if strings.HasSuffix(file, ".age") {
			sourceFile = strings.TrimSuffix(file, ".age")
			targetFile = file
		} else {
			sourceFile = file
			targetFile = file + ".age"
		}

		// Plaintext source must exist to (re-)encrypt.
		if _, err := os.Stat(sourceFile); err != nil {
			if os.IsNotExist(err) {
				log.Debug().Str("file", sourceFile).Msg("Source file doesn't exist, skipping")
				continue
			}
			return fmt.Errorf("failed to stat %s: %w", sourceFile, err)
		}

		// Check whether the target .age file exists.
		if _, err := os.Stat(targetFile); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to stat %s: %w", targetFile, err)
			}
			// Target missing — needs first-time encryption.
			vaultFilesToEncrypt = append(vaultFilesToEncrypt, sourceFile)
			continue
		}

		// Target exists. Re-encrypt when --force or recipient drift detected.
		if ec.force {
			log.Debug().Str("file", targetFile).Msg("--force: re-encrypting existing vault file")
			vaultFilesToEncrypt = append(vaultFilesToEncrypt, sourceFile)
			continue
		}

		stanzaCount, err := fcrypt.CountStanzas(targetFile)
		if err != nil {
			log.Warn().Err(err).Str("file", targetFile).
				Msg("Could not count recipient stanzas; re-encrypting to be safe")
			vaultFilesToEncrypt = append(vaultFilesToEncrypt, sourceFile)
			continue
		}

		if stanzaCount != len(cfg.Age.Recipients) {
			log.Debug().
				Str("file", targetFile).
				Int("stanzas", stanzaCount).
				Int("configured_recipients", len(cfg.Age.Recipients)).
				Msg("Recipient drift detected: re-encrypting vault file")
			vaultFilesToEncrypt = append(vaultFilesToEncrypt, sourceFile)
			continue
		}

		log.Debug().Str("file", targetFile).Msg("Encrypted vault file is up-to-date, skipping")
	}

	// Collect age.files that need encryption.
	// Encrypt when any of:
	//   1. Plaintext dest exists AND encrypted src does not (new file).
	//   2. Plaintext dest is newer than encrypted src (modified file).
	//   3. --force is set.
	//   4. Recipient drift: stanza count in the .age file differs from the
	//      configured recipient count (same heuristic as vault files above).
	ageFilesToEncrypt := []core.AgeFile{}
	for _, af := range cfg.Age.Files {
		destInfo, err := os.Stat(af.Dest)
		if err != nil {
			if os.IsNotExist(err) {
				log.Debug().Str("dest", af.Dest).Msg("Plaintext dest doesn't exist, skipping")
				continue
			}
			return fmt.Errorf("failed to stat %s: %w", af.Dest, err)
		}

		srcInfo, err := os.Stat(af.Src)
		if err != nil {
			if os.IsNotExist(err) {
				// Encrypted file missing — needs encryption.
				ageFilesToEncrypt = append(ageFilesToEncrypt, af)
				continue
			}
			return fmt.Errorf("failed to stat %s: %w", af.Src, err)
		}

		if ec.force {
			log.Debug().Str("src", af.Src).Msg("--force: re-encrypting existing age file")
			ageFilesToEncrypt = append(ageFilesToEncrypt, af)
			continue
		}

		if destInfo.ModTime().After(srcInfo.ModTime()) {
			// Plaintext is newer than encrypted — needs re-encryption.
			ageFilesToEncrypt = append(ageFilesToEncrypt, af)
			continue
		}

		// Mtime is current; check for recipient drift.
		stanzaCount, err := fcrypt.CountStanzas(af.Src)
		if err != nil {
			log.Warn().Err(err).Str("file", af.Src).
				Msg("Could not count recipient stanzas; re-encrypting to be safe")
			ageFilesToEncrypt = append(ageFilesToEncrypt, af)
			continue
		}

		if stanzaCount != len(cfg.Age.Recipients) {
			log.Debug().
				Str("file", af.Src).
				Int("stanzas", stanzaCount).
				Int("configured_recipients", len(cfg.Age.Recipients)).
				Msg("Recipient drift detected: re-encrypting age file")
			ageFilesToEncrypt = append(ageFilesToEncrypt, af)
			continue
		}

		log.Debug().Str("src", af.Src).Str("dest", af.Dest).Msg("Encrypted file is up-to-date, skipping")
	}

	totalToEncrypt := len(vaultFilesToEncrypt) + len(ageFilesToEncrypt)

	if ec.dryRun {
		if totalToEncrypt > 0 {
			log.Error().Msg("Found unencrypted files:")
			for _, file := range vaultFilesToEncrypt {
				log.Error().Str("file", file).Msg("  - vault file needs encryption")
			}
			for _, af := range ageFilesToEncrypt {
				log.Error().Str("dest", af.Dest).Str("src", af.Src).Msg("  - age file needs encryption")
			}
			return fmt.Errorf("found %d unencrypted file(s)", totalToEncrypt)
		}
		log.Info().Msg("All files are encrypted")
		return nil
	}

	if totalToEncrypt == 0 {
		log.Info().Msg("All files are already encrypted")
		return nil
	}

	if len(cfg.Age.Recipients) == 0 {
		return fmt.Errorf("no age recipients configured in mmdot.yaml")
	}

	recipients, err := fcrypt.LoadPublicKeys(cfg.Age.Recipients)
	if err != nil {
		return fmt.Errorf("failed to load public keys: %w", err)
	}

	// Encrypt vault files
	for _, sourceFile := range vaultFilesToEncrypt {
		targetFile := sourceFile + ".age"
		if strings.HasSuffix(sourceFile, ".age") {
			targetFile = sourceFile
			sourceFile = strings.TrimSuffix(sourceFile, ".age")
		}

		log.Info().Str("source", sourceFile).Str("target", targetFile).Msg("Encrypting vault file")
		if err := fcrypt.EncryptFile(sourceFile, targetFile, recipients); err != nil {
			return fmt.Errorf("failed to encrypt %s: %w", sourceFile, err)
		}
		log.Info().Str("file", targetFile).Msg("Vault file encrypted successfully")
	}

	// Encrypt age.files (dest -> src; keep plaintext dest intact)
	for _, af := range ageFilesToEncrypt {
		if err := os.MkdirAll(filepath.Dir(af.Src), 0o755); err != nil {
			return fmt.Errorf("failed to create parent dir for %s: %w", af.Src, err)
		}

		log.Info().Str("source", af.Dest).Str("target", af.Src).Msg("Encrypting age file")
		if err := fcrypt.EncryptFileKeepSource(af.Dest, af.Src, recipients); err != nil {
			return fmt.Errorf("failed to encrypt %s: %w", af.Dest, err)
		}
		log.Info().Str("file", af.Src).Msg("Age file encrypted successfully")
	}

	log.Info().Int("count", totalToEncrypt).Msg("Encryption complete")
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

	decryptedCount := 0

	// Decrypt vault files
	for _, file := range files {
		var sourceFile, targetFile string

		if strings.HasSuffix(file, ".age") {
			sourceFile = file
			targetFile = strings.TrimSuffix(file, ".age")
		} else {
			sourceFile = file + ".age"
			targetFile = file
		}

		if _, err := os.Stat(sourceFile); err != nil {
			if os.IsNotExist(err) {
				log.Debug().Str("file", sourceFile).Msg("Encrypted file doesn't exist, skipping")
				continue
			}
			return fmt.Errorf("failed to stat %s: %w", sourceFile, err)
		}

		if _, err := os.Stat(targetFile); err == nil {
			log.Debug().Str("file", targetFile).Msg("Decrypted file already exists, skipping")
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat %s: %w", targetFile, err)
		}

		log.Info().Str("source", sourceFile).Str("target", targetFile).Msg("Decrypting vault file")
		if err := fcrypt.DecryptFile(sourceFile, targetFile, identity); err != nil {
			return fmt.Errorf("failed to decrypt %s: %w", sourceFile, err)
		}

		if err := os.Remove(sourceFile); err != nil {
			log.Warn().Str("file", sourceFile).Err(err).Msg("Failed to remove encrypted file after decryption")
		}

		decryptedCount++
		log.Info().Str("file", targetFile).Msg("Vault file decrypted successfully")
	}

	// Decrypt age.files (src -> dest, preserve .age file)
	for _, af := range cfg.Age.Files {
		if _, err := os.Stat(af.Src); err != nil {
			if os.IsNotExist(err) {
				log.Debug().Str("src", af.Src).Msg("Encrypted age file doesn't exist, skipping")
				continue
			}
			return fmt.Errorf("failed to stat %s: %w", af.Src, err)
		}

		if _, err := os.Stat(af.Dest); err == nil {
			log.Debug().Str("dest", af.Dest).Msg("Decrypted age file already exists, skipping")
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat %s: %w", af.Dest, err)
		}

		if err := os.MkdirAll(filepath.Dir(af.Dest), 0o755); err != nil {
			return fmt.Errorf("failed to create parent dir for %s: %w", af.Dest, err)
		}

		log.Info().Str("source", af.Src).Str("target", af.Dest).Msg("Decrypting age file")
		if err := fcrypt.DecryptFile(af.Src, af.Dest, identity); err != nil {
			return fmt.Errorf("failed to decrypt %s: %w", af.Src, err)
		}

		if af.Permissions != "" {
			perm, err := core.ParseOctalPermissions(af.Permissions)
			if err != nil {
				return fmt.Errorf("invalid permissions %q for %s: %w", af.Permissions, af.Dest, err)
			}
			if err := os.Chmod(af.Dest, perm); err != nil {
				return fmt.Errorf("failed to set permissions on %s: %w", af.Dest, err)
			}
		}

		relDest, err := filepath.Rel(".", af.Dest)
		if err != nil || strings.HasPrefix(relDest, "..") {
			log.Debug().Str("dest", af.Dest).Msg("Dest outside config dir, skipping gitignore")
		} else if err := ensureGitignored(relDest); err != nil {
			return fmt.Errorf("failed to gitignore %s: %w", af.Dest, err)
		}

		decryptedCount++
		log.Info().Str("file", af.Dest).Msg("Age file decrypted successfully")
	}

	log.Info().Int("count", decryptedCount).Msg("Decryption complete")
	return nil
}

func ensureGitignored(path string) error {
	gitignorePath := ".gitignore"

	data, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .gitignore: %w", err)
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		if line == path {
			return nil
		}
	}

	// Need a leading newline if file exists and doesn't end with one
	prefix := ""
	if len(data) > 0 && data[len(data)-1] != '\n' {
		prefix = "\n"
	}

	f, err := os.OpenFile(gitignorePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open .gitignore for writing: %w", err)
	}

	if _, err := fmt.Fprintf(f, "%s%s\n", prefix, path); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
