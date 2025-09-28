package ssh

import (
	"fmt"
	"strings"
)

// Config represents the main SSH configuration from mmdot.toml
type Config struct {
	ConfigFile    string       `toml:"config_file"`
	Backup        bool         `toml:"backup"`
	PreserveLocal bool         `toml:"preserve_local"`
	Hosts         []HostSource `toml:"hosts"`
}

// HostSource represents a source of SSH hosts (encrypted file or inline)
type HostSource struct {
	Name          string   `toml:"name"`
	EncryptedFile string   `toml:"encrypted_file,omitempty"`
	Hosts         []Host   `toml:"hosts,omitempty"`
	Tags          []string `toml:"tags,omitempty"`
	Priority      int      `toml:"priority"`

	// Per-source encryption settings
	Recipients   []string `toml:"recipients,omitempty"`
	IdentityFile string   `toml:"identity_file,omitempty"`
}

// Host represents a single SSH host configuration
type Host struct {
	Name          string   `toml:"name"`
	Hostname      string   `toml:"hostname"`
	User          string   `toml:"user,omitempty"`
	Port          int      `toml:"port,omitempty"`
	IdentityFile  string   `toml:"identity_file,omitempty"`
	ProxyJump     string   `toml:"proxy_jump,omitempty"`
	ForwardAgent  *bool    `toml:"forward_agent,omitempty"`
	ForwardX11    *bool    `toml:"forward_x11,omitempty"`
	LocalForward  []string `toml:"local_forward,omitempty"`
	RemoteForward []string `toml:"remote_forward,omitempty"`
	Custom        []string `toml:"custom,omitempty"`

	// Internal fields (not from TOML)
	Source   string `toml:"-"` // Track source file/name
	Priority int    `toml:"-"` // From parent HostSource
}

// HostsFile represents the structure of an encrypted/external hosts file
type HostsFile struct {
	Hosts []Host `toml:"hosts"`
}

// ParsedHost represents a host entry parsed from an existing SSH config
type ParsedHost struct {
	Name     string
	Lines    []string
	Comments []string
	Source   string // "local" or "managed:<name>"
}

// String converts a Host to SSH config format
func (h *Host) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Host %s\n", h.Name))

	if h.Hostname != "" {
		sb.WriteString(fmt.Sprintf("    Hostname %s\n", h.Hostname))
	}
	if h.User != "" {
		sb.WriteString(fmt.Sprintf("    User %s\n", h.User))
	}
	if h.Port > 0 {
		sb.WriteString(fmt.Sprintf("    Port %d\n", h.Port))
	}
	if h.IdentityFile != "" {
		sb.WriteString(fmt.Sprintf("    IdentityFile %s\n", h.IdentityFile))
	}
	if h.ProxyJump != "" {
		sb.WriteString(fmt.Sprintf("    ProxyJump %s\n", h.ProxyJump))
	}
	if h.ForwardAgent != nil {
		value := "no"
		if *h.ForwardAgent {
			value = "yes"
		}
		sb.WriteString(fmt.Sprintf("    ForwardAgent %s\n", value))
	}
	if h.ForwardX11 != nil {
		value := "no"
		if *h.ForwardX11 {
			value = "yes"
		}
		sb.WriteString(fmt.Sprintf("    ForwardX11 %s\n", value))
	}
	for _, forward := range h.LocalForward {
		parts := strings.SplitN(forward, ":", 2)
		if len(parts) == 2 {
			sb.WriteString(fmt.Sprintf("    LocalForward %s %s\n", parts[0], parts[1]))
		}
	}
	for _, forward := range h.RemoteForward {
		parts := strings.SplitN(forward, ":", 2)
		if len(parts) == 2 {
			sb.WriteString(fmt.Sprintf("    RemoteForward %s %s\n", parts[0], parts[1]))
		}
	}
	for _, custom := range h.Custom {
		sb.WriteString(fmt.Sprintf("    %s\n", custom))
	}

	// Remove trailing newline for cleaner output when combining hosts
	result := sb.String()
	return strings.TrimSuffix(result, "\n")
}

// Validate checks if the host configuration is valid
func (h *Host) Validate() error {
	if h.Name == "" {
		return fmt.Errorf("host name cannot be empty")
	}
	if h.Hostname == "" {
		return fmt.Errorf("hostname cannot be empty for host %s", h.Name)
	}
	if h.Port < 0 || h.Port > 65535 {
		return fmt.Errorf("invalid port %d for host %s", h.Port, h.Name)
	}
	return nil
}

// HasEncryption returns true if this source has encryption configured
func (hs *HostSource) HasEncryption() bool {
	return hs.EncryptedFile != "" && len(hs.Recipients) > 0
}

// NeedsDecryption returns true if this source needs decryption
func (hs *HostSource) NeedsDecryption() bool {
	return hs.EncryptedFile != ""
}