package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/internal/ssh"
	"github.com/hay-kot/mmdot/pkgs/printer"
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
				Name:  "sync",
				Usage: "Synchronize SSH configurations from all sources",
				Description: `Merges SSH host configurations from all configured sources into your SSH config file.
Preserves local entries and comments while updating managed sections.`,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "show what would change without applying",
					},
				},
				Action: sc.sync,
			},
			{
				Name:  "diff",
				Usage: "Show what would change during sync",
				Description: `Displays the differences between current SSH config and what would be applied
during a sync operation, without making any changes.`,
				Action: sc.diff,
			},
			{
				Name:  "validate",
				Usage: "Validate SSH configuration",
				Description: `Checks all SSH host sources for validity, including:
- Valid TOML syntax
- Required fields present
- No duplicate host names
- Accessible encrypted files and identity keys`,
				Action: sc.validate,
			},
			{
				Name:  "list",
				Usage: "List all configured SSH hosts",
				Description: `Shows all SSH hosts from all configured sources with their priority and source information.`,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:  "tags",
						Usage: "filter hosts by tags",
					},
				},
				Action: sc.list,
			},
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

	log.Debug().Int("recipients", len(recipients)).Msg("found age recipients")

	encryptor := core.NewEncryptor(recipients, "")
	var encrypted int
	var statusItems []printer.StatusListItem
	var warnings []string

	// Process each host source
	for _, source := range cfg.SSH.Hosts {
		// Skip if it has inline hosts (nothing to encrypt)
		if len(source.Hosts) > 0 {
			warnings = append(warnings, fmt.Sprintf("Source %s uses inline hosts (no file to encrypt)", source.Name))
			continue
		}

		// Skip if no encryption configured (no recipients)
		if len(source.Recipients) == 0 {
			warnings = append(warnings, fmt.Sprintf("Source %s has no recipients configured for encryption", source.Name))
			continue
		}

		// Skip if encrypted file already exists and source file doesn't
		if source.EncryptedFile != "" {
			if _, err := os.Stat(source.EncryptedFile); err == nil {
				// Encrypted file exists, check if source file also exists
				sourceFile := strings.TrimSuffix(source.EncryptedFile, ".age")
				if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
					warnings = append(warnings, fmt.Sprintf("Source %s already encrypted (no source file found)", source.Name))
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
			statusItems = append(statusItems, printer.StatusListItem{
				Ok:     false,
				Status: fmt.Sprintf("%s (%d hosts) - failed to remove source file", source.Name, len(hostsFile.Hosts)),
			})
			continue
		}

		statusItems = append(statusItems, printer.StatusListItem{
			Ok:     true,
			Status: fmt.Sprintf("%s (%d hosts) - %s", source.Name, len(hostsFile.Hosts), outputFile),
		})
		encrypted++
	}

	// Now display the nice formatted output at the end
	p := printer.New(os.Stdout)
	p.LineBreak()

	// Display warnings if any
	if len(warnings) > 0 {
		p.List("Warnings:", warnings)
		p.LineBreak()
	}

	// Display encryption results
	if len(statusItems) > 0 {
		p.StatusList("Encryption Results:", statusItems)
		p.LineBreak()
	}

	// Summary
	if encrypted == 0 {
		fmt.Println("No files found to encrypt")
	} else {
		fmt.Printf("Successfully encrypted %d files\n", encrypted)
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

	log.Debug().Msg("processing encrypted SSH host sources")

	var decrypted int
	var statusItems []printer.StatusListItem
	var warnings []string

	// Process each encrypted host source
	for _, source := range cfg.SSH.Hosts {
		// Skip if not encrypted
		if !source.NeedsDecryption() {
			log.Debug().Str("source", source.Name).Msg("source is not encrypted")
			continue
		}

		// Check if encrypted file exists
		if _, err := os.Stat(source.EncryptedFile); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("Source %s encrypted file not found: %s", source.Name, source.EncryptedFile))
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
			warnings = append(warnings, fmt.Sprintf("Source %s has no identity file specified for decryption", source.Name))
			continue
		}

		sourceEncryptor := core.NewEncryptor(nil, source.IdentityFile)

		// Try to decrypt
		if err := sourceEncryptor.DecryptFile(source.EncryptedFile, outputFile); err != nil {
			statusItems = append(statusItems, printer.StatusListItem{
				Ok:     false,
				Status: fmt.Sprintf("%s - failed to decrypt: %v", source.Name, err),
			})
			continue
		}

		// Validate it's a valid hosts file
		var hostsFile ssh.HostsFile
		if _, err := toml.DecodeFile(outputFile, &hostsFile); err != nil {
			statusItems = append(statusItems, printer.StatusListItem{
				Ok:     false,
				Status: fmt.Sprintf("%s - decrypted file is not valid: %v", source.Name, err),
			})
			continue
		}

		// Delete the encrypted file after successful decryption
		if err := os.Remove(source.EncryptedFile); err != nil {
			log.Error().Str("file", source.EncryptedFile).Err(err).Msg("failed to remove encrypted file after decryption")
			statusItems = append(statusItems, printer.StatusListItem{
				Ok:     false,
				Status: fmt.Sprintf("%s (%d hosts) - failed to remove encrypted file", source.Name, len(hostsFile.Hosts)),
			})
			continue
		}

		statusItems = append(statusItems, printer.StatusListItem{
			Ok:     true,
			Status: fmt.Sprintf("%s (%d hosts) - %s", source.Name, len(hostsFile.Hosts), outputFile),
		})
		decrypted++
	}

	// Now display the nice formatted output at the end
	p := printer.New(os.Stdout)
	p.LineBreak()

	// Display warnings if any
	if len(warnings) > 0 {
		p.List("Warnings:", warnings)
		p.LineBreak()
	}

	// Display decryption results
	if len(statusItems) > 0 {
		p.StatusList("Decryption Results:", statusItems)
		p.LineBreak()
	}

	// Summary
	if decrypted == 0 {
		fmt.Println("No encrypted files could be decrypted")
	} else {
		fmt.Printf("Successfully decrypted %d files\n", decrypted)
	}

	return nil
}

func (sc *SSHCmd) sync(ctx context.Context, c *cli.Command) error {
	cfg, err := setupEnv(sc.flags.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if len(cfg.SSH.Hosts) == 0 {
		fmt.Println("No SSH host sources configured in mmdot.toml")
		return nil
	}

	dryRun := c.Bool("dry-run")

	// Load all hosts from sources
	allHosts, err := sc.loadHosts(&cfg.SSH)
	if err != nil {
		return fmt.Errorf("failed to load hosts: %w", err)
	}

	// Parse existing SSH config
	parser := ssh.NewParser(cfg.SSH.PreserveLocal)
	existingHosts, err := parser.ParseFile(cfg.SSH.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to parse existing SSH config: %w", err)
	}

	// Merge configurations
	mergedHosts, err := sc.mergeConfigs(parser, existingHosts, allHosts)
	if err != nil {
		return fmt.Errorf("failed to merge configs: %w", err)
	}

	if dryRun {
		fmt.Println("Dry run mode - showing what would change:")
		return sc.showDiff(existingHosts, mergedHosts)
	}

	// Write the config
	if err := sc.writeConfig(&cfg.SSH, parser, mergedHosts); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Successfully synchronized %d hosts to %s\n", len(allHosts), cfg.SSH.ConfigFile)
	return nil
}

func (sc *SSHCmd) diff(ctx context.Context, c *cli.Command) error {
	cfg, err := setupEnv(sc.flags.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if len(cfg.SSH.Hosts) == 0 {
		fmt.Println("No SSH host sources configured in mmdot.toml")
		return nil
	}

	// Load managed hosts
	allHosts, err := sc.loadHosts(&cfg.SSH)
	if err != nil {
		return fmt.Errorf("failed to load hosts: %w", err)
	}

	// Parse existing config
	parser := ssh.NewParser(cfg.SSH.PreserveLocal)
	existingHosts, err := parser.ParseFile(cfg.SSH.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to parse existing config: %w", err)
	}

	// Merge to see what would change
	mergedHosts, err := sc.mergeConfigs(parser, existingHosts, allHosts)
	if err != nil {
		return fmt.Errorf("failed to merge configs: %w", err)
	}

	return sc.showDiff(existingHosts, mergedHosts)
}

func (sc *SSHCmd) validate(ctx context.Context, c *cli.Command) error {
	cfg, err := setupEnv(sc.flags.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if len(cfg.SSH.Hosts) == 0 {
		fmt.Println("No SSH host sources configured in mmdot.toml")
		return nil
	}

	p := printer.New(os.Stdout)
	p.LineBreak()

	var validationItems []printer.StatusListItem
	var warnings []string

	// Validate basic config
	if cfg.SSH.ConfigFile == "" {
		validationItems = append(validationItems, printer.StatusListItem{
			Ok:     false,
			Status: "SSH config_file cannot be empty",
		})
	}

	// Validate each source
	for i, source := range cfg.SSH.Hosts {
		if source.Name == "" {
			validationItems = append(validationItems, printer.StatusListItem{
				Ok:     false,
				Status: fmt.Sprintf("Host source %d: name cannot be empty", i),
			})
			continue
		}

		if source.EncryptedFile != "" {
			// Validate encrypted source
			if source.IdentityFile == "" {
				validationItems = append(validationItems, printer.StatusListItem{
					Ok:     false,
					Status: fmt.Sprintf("Source %s: identity_file required for encrypted sources", source.Name),
				})
				continue
			}

			// Check if files exist
			if _, err := os.Stat(source.EncryptedFile); err != nil {
				validationItems = append(validationItems, printer.StatusListItem{
					Ok:     false,
					Status: fmt.Sprintf("Source %s: encrypted file not found: %s", source.Name, source.EncryptedFile),
				})
				continue
			}

			if _, err := os.Stat(source.IdentityFile); err != nil {
				validationItems = append(validationItems, printer.StatusListItem{
					Ok:     false,
					Status: fmt.Sprintf("Source %s: identity file not found: %s", source.Name, source.IdentityFile),
				})
				continue
			}

			// Try to decrypt and validate
			encryptor := core.NewEncryptor(source.Recipients, source.IdentityFile)
			decryptedData, err := encryptor.DecryptToBytes(source.EncryptedFile)
			if err != nil {
				validationItems = append(validationItems, printer.StatusListItem{
					Ok:     false,
					Status: fmt.Sprintf("Source %s: failed to decrypt: %v", source.Name, err),
				})
				continue
			}

			var hostsFile ssh.HostsFile
			if err := toml.Unmarshal(decryptedData, &hostsFile); err != nil {
				validationItems = append(validationItems, printer.StatusListItem{
					Ok:     false,
					Status: fmt.Sprintf("Source %s: invalid TOML in encrypted file: %v", source.Name, err),
				})
				continue
			}

			if err := ssh.ValidateHosts(hostsFile.Hosts); err != nil {
				validationItems = append(validationItems, printer.StatusListItem{
					Ok:     false,
					Status: fmt.Sprintf("Source %s: invalid hosts: %v", source.Name, err),
				})
				continue
			}

			validationItems = append(validationItems, printer.StatusListItem{
				Ok:     true,
				Status: fmt.Sprintf("Source %s: valid encrypted source (%d hosts)", source.Name, len(hostsFile.Hosts)),
			})

		} else if len(source.Hosts) > 0 {
			// Validate inline hosts
			if err := ssh.ValidateHosts(source.Hosts); err != nil {
				validationItems = append(validationItems, printer.StatusListItem{
					Ok:     false,
					Status: fmt.Sprintf("Source %s: invalid hosts: %v", source.Name, err),
				})
				continue
			}

			validationItems = append(validationItems, printer.StatusListItem{
				Ok:     true,
				Status: fmt.Sprintf("Source %s: valid inline source (%d hosts)", source.Name, len(source.Hosts)),
			})

		} else {
			validationItems = append(validationItems, printer.StatusListItem{
				Ok:     false,
				Status: fmt.Sprintf("Source %s: must specify either encrypted_file or inline hosts", source.Name),
			})
		}
	}

	// Try to load all hosts
	allHosts, err := sc.loadHosts(&cfg.SSH)
	if err != nil {
		validationItems = append(validationItems, printer.StatusListItem{
			Ok:     false,
			Status: fmt.Sprintf("Failed to load hosts: %v", err),
		})
	} else {
		validationItems = append(validationItems, printer.StatusListItem{
			Ok:     true,
			Status: fmt.Sprintf("Successfully loaded %d total hosts", len(allHosts)),
		})
	}

	// Display warnings
	if len(warnings) > 0 {
		p.List("Warnings:", warnings)
		p.LineBreak()
	}

	// Display validation results
	p.StatusList("Validation Results:", validationItems)

	// Count successes and failures
	var successes, failures int
	for _, item := range validationItems {
		if item.Ok {
			successes++
		} else {
			failures++
		}
	}

	p.LineBreak()
	if failures == 0 {
		fmt.Println("✅ All validations passed")
	} else {
		fmt.Printf("❌ %d validation(s) failed, %d passed\n", failures, successes)
		return fmt.Errorf("validation failed")
	}

	return nil
}

func (sc *SSHCmd) list(ctx context.Context, c *cli.Command) error {
	cfg, err := setupEnv(sc.flags.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if len(cfg.SSH.Hosts) == 0 {
		fmt.Println("No SSH host sources configured in mmdot.toml")
		return nil
	}

	// Load all hosts
	allHosts, err := sc.loadHosts(&cfg.SSH)
	if err != nil {
		return fmt.Errorf("failed to load hosts: %w", err)
	}

	if len(allHosts) == 0 {
		fmt.Println("No hosts found in configured sources")
		return nil
	}

	// Filter by tags if specified
	tagFilter := c.StringSlice("tags")
	var filteredHosts []ssh.Host
	if len(tagFilter) > 0 {
		// Note: Tag filtering would need to be implemented in the Host struct
		// For now, we'll show all hosts
		filteredHosts = allHosts
	} else {
		filteredHosts = allHosts
	}

	p := printer.New(os.Stdout)
	p.LineBreak()

	var hostItems []string
	for _, host := range filteredHosts {
		hostInfo := fmt.Sprintf("%s -> %s", host.Name, host.Hostname)
		if host.User != "" {
			hostInfo = fmt.Sprintf("%s -> %s@%s", host.Name, host.User, host.Hostname)
		}
		if host.Port > 0 && host.Port != 22 {
			hostInfo = fmt.Sprintf("%s:%d", hostInfo, host.Port)
		}
		hostInfo = fmt.Sprintf("%s (source: %s, priority: %d)", hostInfo, host.Source, host.Priority)
		hostItems = append(hostItems, hostInfo)
	}

	p.List(fmt.Sprintf("SSH Hosts (%d total):", len(filteredHosts)), hostItems)

	return nil
}

func (sc *SSHCmd) showDiff(existing, merged []ssh.ParsedHost) error {
	p := printer.New(os.Stdout)
	p.LineBreak()

	// Create maps for easy lookup
	existingMap := make(map[string]ssh.ParsedHost)
	for _, host := range existing {
		existingMap[host.Name] = host
	}

	mergedMap := make(map[string]ssh.ParsedHost)
	for _, host := range merged {
		mergedMap[host.Name] = host
	}

	var added, modified, removed []string

	// Find added and modified hosts
	for name, mergedHost := range mergedMap {
		if existingHost, exists := existingMap[name]; exists {
			// Check if modified (simple string comparison of lines)
			if !equalStringSlices(existingHost.Lines, mergedHost.Lines) {
				modified = append(modified, fmt.Sprintf("%s (source: %s)", name, mergedHost.Source))
			}
		} else {
			added = append(added, fmt.Sprintf("%s (source: %s)", name, mergedHost.Source))
		}
	}

	// Find removed hosts (only managed ones get "removed")
	for name, existingHost := range existingMap {
		if _, exists := mergedMap[name]; !exists && strings.HasPrefix(existingHost.Source, "managed:") {
			removed = append(removed, fmt.Sprintf("%s (was source: %s)", name, existingHost.Source))
		}
	}

	// Display changes
	if len(added) > 0 {
		p.List("Hosts to be added:", added)
		p.LineBreak()
	}

	if len(modified) > 0 {
		p.List("Hosts to be modified:", modified)
		p.LineBreak()
	}

	if len(removed) > 0 {
		p.List("Hosts to be removed:", removed)
		p.LineBreak()
	}

	if len(added)+len(modified)+len(removed) == 0 {
		fmt.Println("No changes would be made")
	} else {
		fmt.Printf("Summary: %d added, %d modified, %d removed\n", len(added), len(modified), len(removed))
	}

	return nil
}

// Helper functions for SSH operations

func (sc *SSHCmd) loadHosts(config *ssh.Config) ([]ssh.Host, error) {
	var allHosts []ssh.Host

	for _, source := range config.Hosts {
		hosts, err := sc.loadSourceHosts(&source)
		if err != nil {
			return nil, fmt.Errorf("failed to load hosts from source %q: %w", source.Name, err)
		}

		// Set priority and source for all hosts
		for i := range hosts {
			hosts[i].Priority = source.Priority
			hosts[i].Source = source.Name
		}

		allHosts = append(allHosts, hosts...)
	}

	// Sort by priority (higher first)
	ssh.SortHostsByPriority(allHosts)

	// Deduplicate based on priority
	deduplicated := ssh.DeduplicateHosts(allHosts)

	// Validate final host list
	if err := ssh.ValidateHosts(deduplicated); err != nil {
		return nil, fmt.Errorf("host validation failed: %w", err)
	}

	return deduplicated, nil
}

func (sc *SSHCmd) loadSourceHosts(source *ssh.HostSource) ([]ssh.Host, error) {
	// If encrypted file is specified, decrypt and load from it
	if source.EncryptedFile != "" {
		return sc.loadEncryptedHosts(source)
	}

	// Otherwise, return inline hosts
	return source.Hosts, nil
}

func (sc *SSHCmd) loadEncryptedHosts(source *ssh.HostSource) ([]ssh.Host, error) {
	if source.IdentityFile == "" {
		return nil, fmt.Errorf("identity_file required for encrypted source %q", source.Name)
	}

	// Create encryptor for decryption
	encryptor := core.NewEncryptor(source.Recipients, source.IdentityFile)

	// Decrypt the file to bytes
	decryptedData, err := encryptor.DecryptToBytes(source.EncryptedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt hosts file %q: %w", source.EncryptedFile, err)
	}

	// Parse TOML data
	var hostsFile ssh.HostsFile
	if err := toml.Unmarshal(decryptedData, &hostsFile); err != nil {
		return nil, fmt.Errorf("failed to parse decrypted hosts TOML: %w", err)
	}

	return hostsFile.Hosts, nil
}

func (sc *SSHCmd) mergeConfigs(parser *ssh.Parser, existingHosts []ssh.ParsedHost, managedHosts []ssh.Host) ([]ssh.ParsedHost, error) {
	// Group managed hosts by source
	hostsBySource := make(map[string][]ssh.Host)
	for _, host := range managedHosts {
		hostsBySource[host.Source] = append(hostsBySource[host.Source], host)
	}

	// Merge each source
	result := existingHosts
	for sourceName, hosts := range hostsBySource {
		result = parser.MergeHosts(result, hosts, sourceName)
	}

	return result, nil
}

func (sc *SSHCmd) writeConfig(config *ssh.Config, parser *ssh.Parser, hosts []ssh.ParsedHost) error {
	// Create backup if requested
	if config.Backup {
		if err := sc.createBackup(config.ConfigFile); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Write atomically using temp file + rename
	return sc.writeAtomic(config.ConfigFile, parser, hosts)
}

func (sc *SSHCmd) createBackup(configPath string) error {
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// No file to backup
		return nil
	}

	// Create backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.backup-%s", configPath, timestamp)

	// Copy file
	input, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := os.WriteFile(backupPath, input, 0600); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	return nil
}

func (sc *SSHCmd) writeAtomic(configPath string, parser *ssh.Parser, hosts []ssh.ParsedHost) error {
	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create SSH config directory: %w", err)
	}

	// Create temp file in same directory
	tempFile, err := os.CreateTemp(dir, ".ssh-config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
	}()

	// Write config to temp file
	if err := parser.WriteConfig(tempFile, hosts, true); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Sync to disk
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	// Close temp file
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Set correct permissions
	if err := os.Chmod(tempFile.Name(), 0600); err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile.Name(), configPath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
