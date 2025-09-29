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
		},
	}

	app.Commands = append(app.Commands, cmd)
	return app
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

	if err := os.WriteFile(backupPath, input, 0o600); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	return nil
}

func (sc *SSHCmd) writeAtomic(configPath string, parser *ssh.Parser, hosts []ssh.ParsedHost) error {
	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
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
	if err := os.Chmod(tempFile.Name(), 0o600); err != nil {
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
