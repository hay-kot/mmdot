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
	cleanDry  bool
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
					Usage:       "force re-encryption of every managed .age file with the current recipients (escape hatch for swapped-but-same-count recipients, or when rotation detection misbehaves)",
					Destination: &ec.force,
				},
			},
			Action: ec.encrypt,
		},
		{
			Name:  "clean",
			Usage: "remove decrypted plaintext artifacts, keeping .age files intact",
			Description: `Removes plaintext copies of managed secret files when an
encrypted .age counterpart exists.

Useful when decommissioning a machine or pruning a local checkout. The
.age (encrypted) versions are never touched; only the decrypted
plaintext is removed. Files whose encrypted counterpart is missing are
left alone so you don't lose data.`,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:        "dry-run",
					Usage:       "show what would be removed without deleting anything",
					Destination: &ec.cleanDry,
				},
			},
			Action: ec.clean,
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

	// Collect vault files that need encryption
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

		if _, err := os.Stat(sourceFile); err != nil {
			if os.IsNotExist(err) {
				log.Debug().Str("file", sourceFile).Msg("Source file doesn't exist, skipping")
				continue
			}
			return fmt.Errorf("failed to stat %s: %w", sourceFile, err)
		}

		if _, err := os.Stat(targetFile); err == nil {
			log.Debug().Str("file", targetFile).Msg("Encrypted file already exists, skipping")
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat %s: %w", targetFile, err)
		}

		vaultFilesToEncrypt = append(vaultFilesToEncrypt, sourceFile)
	}

	// Collect age.files that need encryption.
	// Only encrypt when:
	//   1. Plaintext dest exists AND encrypted src does not (new file)
	//   2. Plaintext dest is newer than encrypted src (modified file)
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
				// Encrypted file missing — needs encryption
				ageFilesToEncrypt = append(ageFilesToEncrypt, af)
				continue
			}
			return fmt.Errorf("failed to stat %s: %w", af.Src, err)
		}

		if destInfo.ModTime().After(srcInfo.ModTime()) {
			// Plaintext is newer than encrypted — needs re-encryption
			ageFilesToEncrypt = append(ageFilesToEncrypt, af)
			continue
		}

		log.Debug().Str("src", af.Src).Str("dest", af.Dest).Msg("Encrypted file is up-to-date, skipping")
	}

	// Collect existing .age files whose recipient stanza count no longer
	// matches the configured recipient list (or all of them, when --rotate is
	// passed). These need to be decrypted and re-encrypted so newly added
	// keys can read them.
	filesToRotate, err := ec.collectRotationCandidates(cfg)
	if err != nil {
		return err
	}

	totalToEncrypt := len(vaultFilesToEncrypt) + len(ageFilesToEncrypt)
	totalWork := totalToEncrypt + len(filesToRotate)

	if ec.dryRun {
		if totalWork > 0 {
			log.Error().Msg("Found files needing encryption work:")
			for _, file := range vaultFilesToEncrypt {
				log.Error().Str("file", file).Msg("  - vault file needs encryption")
			}
			for _, af := range ageFilesToEncrypt {
				log.Error().Str("dest", af.Dest).Str("src", af.Src).Msg("  - age file needs encryption")
			}
			for _, file := range filesToRotate {
				log.Error().Str("file", file).Msg("  - .age file needs recipient rotation")
			}
			return fmt.Errorf("found %d file(s) needing encryption work", totalWork)
		}
		log.Info().Msg("All files are encrypted and up-to-date with current recipients")
		return nil
	}

	if totalWork == 0 {
		log.Info().Msg("All files are already encrypted and up-to-date with current recipients")
		return nil
	}

	if len(cfg.Age.Recipients) == 0 {
		return fmt.Errorf("no age recipients configured in mmdot.yaml")
	}

	recipients, err := fcrypt.LoadPublicKeys(cfg.Age.Recipients)
	if err != nil {
		return fmt.Errorf("failed to load public keys: %w", err)
	}

	// Rotation requires the identity so we can decrypt then re-encrypt.
	if len(filesToRotate) > 0 {
		identity, err := cfg.Age.ReadIdentity()
		if err != nil {
			return fmt.Errorf("rotation requires age identity: %w", err)
		}
		for _, path := range filesToRotate {
			log.Info().Str("file", path).Msg("Rotating recipients")
			if err := fcrypt.RecryptFile(path, identity, recipients); err != nil {
				return fmt.Errorf("rotate %s: %w", path, err)
			}
		}
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

	log.Info().
		Int("encrypted", totalToEncrypt).
		Int("rotated", len(filesToRotate)).
		Msg("Encryption complete")
	return nil
}

// collectRotationCandidates returns the list of existing .age files whose
// recipient stanza count no longer matches the configured recipients (or all
// existing .age files when --force is set).
//
// Stanza counting is a heuristic that catches the common case of adding or
// removing a recipient. Same-count swaps, or any bug in the heuristic, can
// be worked around by re-running with --force, which unconditionally
// re-encrypts every managed .age file.
func (ec *EncryptCmd) collectRotationCandidates(cfg core.ConfigFile) ([]string, error) {
	want := len(cfg.Age.Recipients)
	var candidates []string

	check := func(path string) error {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if info.IsDir() {
			return nil
		}
		if ec.force {
			candidates = append(candidates, path)
			return nil
		}
		got, err := fcrypt.CountRecipientStanzas(path)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
		if got != want {
			log.Debug().Str("file", path).Int("have", got).Int("want", want).Msg("recipient count mismatch")
			candidates = append(candidates, path)
		}
		return nil
	}

	for _, vf := range cfg.EncryptedFiles() {
		path := vf
		if !strings.HasSuffix(path, ".age") {
			path += ".age"
		}
		if err := check(path); err != nil {
			return nil, err
		}
	}
	for _, af := range cfg.Age.Files {
		if err := check(af.Src); err != nil {
			return nil, err
		}
	}
	return candidates, nil
}

// clean removes plaintext copies of managed secret files when the encrypted
// .age counterpart exists. Encrypted files are never touched.
func (ec *EncryptCmd) clean(ctx context.Context, cmd *cli.Command) error {
	cfg, err := core.SetupEnv(ec.coreFlags.ConfigFilePath)
	if err != nil {
		return err
	}

	type target struct {
		label     string
		plaintext string
		encrypted string
	}
	var targets []target

	for _, vf := range cfg.EncryptedFiles() {
		plaintext := strings.TrimSuffix(vf, ".age")
		encrypted := plaintext + ".age"
		targets = append(targets, target{"vault", plaintext, encrypted})
	}
	for _, af := range cfg.Age.Files {
		targets = append(targets, target{"age file", af.Dest, af.Src})
	}

	removed := 0
	for _, t := range targets {
		if _, err := os.Stat(t.plaintext); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat %s: %w", t.plaintext, err)
		}
		if _, err := os.Stat(t.encrypted); err != nil {
			if os.IsNotExist(err) {
				log.Warn().
					Str("plaintext", t.plaintext).
					Str("encrypted", t.encrypted).
					Msg("Encrypted counterpart missing; skipping to avoid data loss")
				continue
			}
			return fmt.Errorf("stat %s: %w", t.encrypted, err)
		}

		if ec.cleanDry {
			log.Info().Str("type", t.label).Str("file", t.plaintext).Msg("would remove plaintext")
			removed++
			continue
		}

		log.Info().Str("type", t.label).Str("file", t.plaintext).Msg("removing plaintext")
		if err := os.Remove(t.plaintext); err != nil {
			return fmt.Errorf("remove %s: %w", t.plaintext, err)
		}
		removed++
	}

	verb := "removed"
	if ec.cleanDry {
		verb = "would remove"
	}
	log.Info().Int("count", removed).Msgf("Clean complete (%s plaintext file(s))", verb)
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

		// Note: the encrypted .age file is intentionally left in place. It's
		// committed to the repo and removing it here would force users to
		// re-run `encrypt` after every decrypt. Use `mmdot clean` to drop
		// plaintext copies when decommissioning a machine.

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
