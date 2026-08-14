package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"filippo.io/age"
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
			Usage: "encrypt managed secret files",
			Description: `Encrypts configured secret files using age encryption.

The command will:
- Create or update .age files for managed secrets
- Remove plaintext vault variable files after encryption
- Keep plaintext age.files destinations after encryption
- Rotate existing .age files when the configured recipient count changes
- Skip files whose plaintext already matches the encrypted copy

Deciding whether an .age file is current requires decrypting it, so the
configured age identity (private key) must be readable whenever both the
plaintext and the .age file are present.

Encrypted files use the age format and can only be decrypted with a
corresponding age identity (private key).`,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:        "dry-run",
					Usage:       "check if files need encryption without encrypting them",
					Destination: &ec.dryRun,
				},
				&cli.BoolFlag{
					Name:        "force",
					Usage:       "re-encrypt all managed .age files with the current recipients",
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
			Usage: "decrypt managed secret files",
			Description: `Decrypts configured .age files.

The command will:
- Use your configured age identity (private key) for decryption
- Restore plaintext copies of managed secret files
- Keep the .age encrypted files in place
- Skip files that are already decrypted, warning when the existing
  plaintext differs from the encrypted copy

Use this when you need to edit secret files or when setting up a new machine
from encrypted configuration files. Run 'mmdot clean' to remove plaintext
copies later.`,
			Action: ec.decrypt,
		},
	}

	app.Commands = append(app.Commands, cmds...)
	return app
}

// managedSecret pairs a plaintext file with its encrypted .age counterpart.
type managedSecret struct {
	label     string
	plaintext string
	encrypted string
	// removePlaintext marks vault variable files, whose plaintext copy is
	// deleted once encrypted. age.files destinations are kept because they are
	// the live file the system reads.
	removePlaintext bool
}

// managedSecrets returns every plaintext/.age pair mmdot manages.
func managedSecrets(cfg core.ConfigFile) []managedSecret {
	secrets := make([]managedSecret, 0, len(cfg.Variables.VarFiles)+len(cfg.Age.Files))

	for _, vf := range cfg.EncryptedFiles() {
		plaintext := strings.TrimSuffix(vf, ".age")
		secrets = append(secrets, managedSecret{
			label:           "vault file",
			plaintext:       plaintext,
			encrypted:       plaintext + ".age",
			removePlaintext: true,
		})
	}

	for _, af := range cfg.Age.Files {
		secrets = append(secrets, managedSecret{
			label:     "age file",
			plaintext: af.Dest,
			encrypted: af.Src,
		})
	}

	return secrets
}

type encryptReason string

const (
	reasonNotEncrypted  encryptReason = "no .age file yet"
	reasonPlaintextEdit encryptReason = "plaintext differs from encrypted copy"
	reasonRecipients    encryptReason = "recipient set changed"
	reasonForced        encryptReason = "forced re-encrypt"
)

// encryptAction is a single unit of work produced by planEncryption.
type encryptAction struct {
	secret managedSecret
	reason encryptReason
	// fromPlaintext is false when no plaintext copy exists on this machine and
	// the .age file can only be rewritten by decrypting and re-encrypting its
	// current contents.
	fromPlaintext bool
}

// identityLoader resolves the configured age identity on first use. Encrypt
// only needs the private key when it has to read existing ciphertext, so
// creating the first .age file on a machine without the key still works.
type identityLoader func() (age.Identity, error)

func newIdentityLoader(cfg core.ConfigFile) identityLoader {
	var (
		once     sync.Once
		identity age.Identity
		err      error
	)
	return func() (age.Identity, error) {
		once.Do(func() { identity, err = cfg.Age.ReadIdentity() })
		return identity, err
	}
}

func (ec *EncryptCmd) encrypt(ctx context.Context, cmd *cli.Command) error {
	cfg, err := core.SetupEnv(ec.coreFlags.ConfigFilePath)
	if err != nil {
		return err
	}

	identity := newIdentityLoader(cfg)

	actions, err := ec.planEncryption(cfg, identity)
	if err != nil {
		return err
	}

	if ec.dryRun {
		if len(actions) == 0 {
			log.Info().Msg("All files are encrypted and up-to-date with current recipients")
			return nil
		}

		log.Error().Msg("Found files needing encryption work:")
		for _, action := range actions {
			event := log.Error().
				Str("file", action.secret.encrypted).
				Str("reason", string(action.reason))
			if action.fromPlaintext {
				event = event.Str("source", action.secret.plaintext)
			}
			event.Msgf("  - %s needs encryption", action.secret.label)
		}
		return fmt.Errorf("found %d file(s) needing encryption work", len(actions))
	}

	if len(actions) == 0 {
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

	encrypted, rotated := 0, 0
	for _, action := range actions {
		secret := action.secret

		if !action.fromPlaintext {
			id, err := identity()
			if err != nil {
				return fmt.Errorf("re-encrypting %s requires the age identity: %w", secret.encrypted, err)
			}

			log.Info().Str("file", secret.encrypted).Str("reason", string(action.reason)).Msg("Re-encrypting for current recipients")
			if err := fcrypt.RecryptFile(secret.encrypted, id, recipients); err != nil {
				return fmt.Errorf("rotate %s: %w", secret.encrypted, err)
			}
			rotated++
			continue
		}

		if err := os.MkdirAll(filepath.Dir(secret.encrypted), 0o755); err != nil {
			return fmt.Errorf("failed to create parent dir for %s: %w", secret.encrypted, err)
		}

		encryptFile := fcrypt.EncryptFileKeepSource
		if secret.removePlaintext {
			encryptFile = fcrypt.EncryptFile
		}

		log.Info().
			Str("source", secret.plaintext).
			Str("target", secret.encrypted).
			Str("reason", string(action.reason)).
			Msgf("Encrypting %s", secret.label)

		if err := encryptFile(secret.plaintext, secret.encrypted, recipients); err != nil {
			return fmt.Errorf("failed to encrypt %s: %w", secret.plaintext, err)
		}
		encrypted++
		log.Info().Str("type", secret.label).Str("file", secret.encrypted).Msg("Encrypted successfully")
	}

	log.Info().
		Int("encrypted", encrypted).
		Int("rotated", rotated).
		Msg("Encryption complete")
	return nil
}

// planEncryption decides what encrypt has to do, without touching any file.
//
// Whether an .age file is current is decided by content, never by existence or
// mtime: the documented decrypt -> edit -> encrypt workflow leaves both files in
// place, so an existence check silently discards every edit.
func (ec *EncryptCmd) planEncryption(cfg core.ConfigFile, identity identityLoader) ([]encryptAction, error) {
	wantRecipients := len(cfg.Age.Recipients)
	var actions []encryptAction

	for _, secret := range managedSecrets(cfg) {
		hasPlaintext, err := fileExists(secret.plaintext)
		if err != nil {
			return nil, err
		}

		hasEncrypted, err := fileExists(secret.encrypted)
		if err != nil {
			return nil, err
		}

		switch {
		case !hasPlaintext && !hasEncrypted:
			log.Debug().Str("file", secret.plaintext).Msg("Neither plaintext nor .age file exists, skipping")

		case !hasPlaintext:
			// Nothing to encrypt from; the only possible work is rewriting the
			// .age file for the current recipients.
			switch {
			case ec.force:
				actions = append(actions, encryptAction{secret: secret, reason: reasonForced})
			default:
				stale, err := recipientsStale(secret.encrypted, wantRecipients)
				if err != nil {
					return nil, err
				}
				if stale {
					actions = append(actions, encryptAction{secret: secret, reason: reasonRecipients})
				}
			}

		case !hasEncrypted:
			actions = append(actions, encryptAction{secret: secret, reason: reasonNotEncrypted, fromPlaintext: true})

		case ec.force:
			actions = append(actions, encryptAction{secret: secret, reason: reasonForced, fromPlaintext: true})

		default:
			id, err := identity()
			if err != nil {
				return nil, fmt.Errorf("checking %s for changes requires the age identity: %w", secret.encrypted, err)
			}

			differs, err := plaintextDiffers(secret, id)
			if err != nil {
				return nil, err
			}
			if differs {
				actions = append(actions, encryptAction{secret: secret, reason: reasonPlaintextEdit, fromPlaintext: true})
				continue
			}

			stale, err := recipientsStale(secret.encrypted, wantRecipients)
			if err != nil {
				return nil, err
			}
			if stale {
				actions = append(actions, encryptAction{secret: secret, reason: reasonRecipients, fromPlaintext: true})
			}
		}
	}

	return actions, nil
}

// plaintextDiffers reports whether a secret's plaintext copy has diverged from
// the contents of its .age counterpart.
//
// age encryption is non-deterministic — each run picks a fresh random file key,
// so encrypting identical plaintext twice produces different ciphertext.
// Comparing ciphertext bytes (or hashes of them) would mark every file as
// changed, so the comparison has to happen on decrypted plaintext.
func plaintextDiffers(secret managedSecret, identity age.Identity) (bool, error) {
	current, err := os.ReadFile(secret.plaintext)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", secret.plaintext, err)
	}

	f, err := os.Open(secret.encrypted)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", secret.encrypted, err)
	}
	defer func() { _ = f.Close() }()

	var stored bytes.Buffer
	if err := fcrypt.DecryptReader(f, &stored, identity); err != nil {
		return false, fmt.Errorf("decrypt %s to compare it against %s (use --force to re-encrypt from plaintext without comparing): %w", secret.encrypted, secret.plaintext, err)
	}

	return !bytes.Equal(stored.Bytes(), current), nil
}

// recipientsStale reports whether an existing .age file's recipient stanza count
// no longer matches the configured recipient list.
//
// Stanza counting is a heuristic that catches the common case of adding or
// removing a recipient. Same-count swaps can be handled by re-running with
// --force.
func recipientsStale(path string, want int) (bool, error) {
	got, err := fcrypt.CountRecipientStanzas(path)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}
	if got != want {
		log.Debug().Str("file", path).Int("have", got).Int("want", want).Msg("recipient count mismatch")
		return true, nil
	}
	return false, nil
}

func fileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat %s: %w", path, err)
	}
	return true, nil
}

// clean removes plaintext copies of managed secret files when the encrypted
// .age counterpart exists. Encrypted files are never touched.
func (ec *EncryptCmd) clean(ctx context.Context, cmd *cli.Command) error {
	cfg, err := core.SetupEnv(ec.coreFlags.ConfigFilePath)
	if err != nil {
		return err
	}

	removed := 0
	for _, secret := range managedSecrets(cfg) {
		hasPlaintext, err := fileExists(secret.plaintext)
		if err != nil {
			return err
		}
		if !hasPlaintext {
			continue
		}

		hasEncrypted, err := fileExists(secret.encrypted)
		if err != nil {
			return err
		}
		if !hasEncrypted {
			log.Warn().
				Str("plaintext", secret.plaintext).
				Str("encrypted", secret.encrypted).
				Msg("Encrypted counterpart missing; skipping to avoid data loss")
			continue
		}

		if ec.cleanDry {
			log.Info().Str("type", secret.label).Str("file", secret.plaintext).Msg("would remove plaintext")
			removed++
			continue
		}

		log.Info().Str("type", secret.label).Str("file", secret.plaintext).Msg("removing plaintext")
		if err := os.Remove(secret.plaintext); err != nil {
			return fmt.Errorf("remove %s: %w", secret.plaintext, err)
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

		hasSource, err := fileExists(sourceFile)
		if err != nil {
			return err
		}
		if !hasSource {
			log.Debug().Str("file", sourceFile).Msg("Encrypted file doesn't exist, skipping")
			continue
		}

		hasTarget, err := fileExists(targetFile)
		if err != nil {
			return err
		}
		if hasTarget {
			warnOnDivergence(managedSecret{label: "vault file", plaintext: targetFile, encrypted: sourceFile}, identity)
			continue
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
		hasSource, err := fileExists(af.Src)
		if err != nil {
			return err
		}
		if !hasSource {
			log.Debug().Str("src", af.Src).Msg("Encrypted age file doesn't exist, skipping")
			continue
		}

		hasDest, err := fileExists(af.Dest)
		if err != nil {
			return err
		}
		if hasDest {
			warnOnDivergence(managedSecret{label: "age file", plaintext: af.Dest, encrypted: af.Src}, identity)
			continue
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

// warnOnDivergence surfaces a plaintext copy that no longer matches the .age
// file decrypt is about to skip.
//
// Skipping is deliberate — it protects local edits from being clobbered — but
// staying silent about a mismatch hides it until something downstream renders
// the wrong value, so the divergence is reported instead of assumed benign.
func warnOnDivergence(secret managedSecret, identity age.Identity) {
	differs, err := plaintextDiffers(secret, identity)
	if err != nil {
		log.Warn().Err(err).
			Str("plaintext", secret.plaintext).
			Str("encrypted", secret.encrypted).
			Msg("Could not compare the existing plaintext against the .age file")
		return
	}

	if differs {
		log.Warn().
			Str("plaintext", secret.plaintext).
			Str("encrypted", secret.encrypted).
			Msg("Existing plaintext differs from the .age file and was left untouched; run 'mmdot encrypt' to save these edits, or delete the plaintext and decrypt again to discard them")
		return
	}

	log.Debug().Str("file", secret.plaintext).Msg("Decrypted file already exists and matches, skipping")
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
