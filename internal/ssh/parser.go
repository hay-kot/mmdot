package ssh

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const (
	markerPrefix = "# === BEGIN MMDOT MANAGED:"
	markerSuffix = "# === END MMDOT MANAGED:"
)

// Parser handles reading and writing SSH config files
type Parser struct {
	preserveLocal bool
}

// NewParser creates a new SSH config parser
func NewParser(preserveLocal bool) *Parser {
	return &Parser{
		preserveLocal: preserveLocal,
	}
}

// ParseFile reads an SSH config file and extracts hosts
func (p *Parser) ParseFile(path string) ([]ParsedHost, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, return empty list
			return []ParsedHost{}, nil
		}
		return nil, fmt.Errorf("failed to open SSH config: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	return p.Parse(file)
}

// Parse reads SSH config from a reader
func (p *Parser) Parse(r io.Reader) ([]ParsedHost, error) {
	var hosts []ParsedHost
	scanner := bufio.NewScanner(r)

	var currentHost *ParsedHost
	var inManagedSection bool
	var managedSectionName string
	var comments []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check for managed section markers
		if strings.HasPrefix(trimmed, markerPrefix) {
			inManagedSection = true
			managedSectionName = strings.TrimSpace(strings.TrimPrefix(trimmed, markerPrefix))
			managedSectionName = strings.TrimSuffix(managedSectionName, "===")
			managedSectionName = strings.TrimSpace(managedSectionName)
			continue
		}

		if strings.HasPrefix(trimmed, markerSuffix) {
			inManagedSection = false
			managedSectionName = ""
			continue
		}

		// Skip content in managed sections if we're preserving local
		if inManagedSection && p.preserveLocal {
			continue
		}

		// Handle comments
		if strings.HasPrefix(trimmed, "#") {
			comments = append(comments, line)
			continue
		}

		// Skip empty lines
		if trimmed == "" {
			if currentHost != nil {
				currentHost.Lines = append(currentHost.Lines, line)
			}
			continue
		}

		// Check for Host directive
		if strings.HasPrefix(strings.ToLower(trimmed), "host ") {
			// Save previous host if exists
			if currentHost != nil {
				hosts = append(hosts, *currentHost)
			}

			// Start new host
			hostName := strings.TrimSpace(trimmed[5:])
			source := "local"
			if inManagedSection {
				source = fmt.Sprintf("managed:%s", managedSectionName)
			}

			currentHost = &ParsedHost{
				Name:     hostName,
				Lines:    []string{line},
				Comments: comments,
				Source:   source,
			}
			comments = nil
		} else if currentHost != nil {
			// Add line to current host
			currentHost.Lines = append(currentHost.Lines, line)
		}
	}

	// Don't forget the last host
	if currentHost != nil {
		hosts = append(hosts, *currentHost)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading SSH config: %w", err)
	}

	return hosts, nil
}

// MergeHosts merges managed hosts with existing config
func (p *Parser) MergeHosts(existing []ParsedHost, managed []Host, sourceName string) []ParsedHost {
	var result []ParsedHost

	// Create a map of managed host names for quick lookup
	managedMap := make(map[string]Host)
	for _, h := range managed {
		managedMap[h.Name] = h
	}

	// Process existing hosts
	for _, existingHost := range existing {
		// Skip managed hosts from this source (they'll be replaced)
		if existingHost.Source == fmt.Sprintf("managed:%s", sourceName) {
			continue
		}

		// Skip if this host is being replaced by a higher priority managed host
		if _, isManaged := managedMap[existingHost.Name]; isManaged && existingHost.Source != "local" {
			continue
		}

		// Keep local hosts if preserving
		if p.preserveLocal && existingHost.Source == "local" {
			result = append(result, existingHost)
		}
	}

	// Add managed hosts
	for _, managedHost := range managed {
		parsed := ParsedHost{
			Name:   managedHost.Name,
			Lines:  strings.Split(managedHost.String(), "\n"),
			Source: fmt.Sprintf("managed:%s", sourceName),
		}
		result = append(result, parsed)
	}

	return result
}

// WriteConfig writes hosts to an SSH config file
func (p *Parser) WriteConfig(w io.Writer, hosts []ParsedHost, groupBySource bool) error {
	if groupBySource {
		return p.writeGroupedConfig(w, hosts)
	}
	return p.writeLinearConfig(w, hosts)
}

// writeGroupedConfig writes config with managed sections grouped
func (p *Parser) writeGroupedConfig(w io.Writer, hosts []ParsedHost) error {
	// Group hosts by source
	grouped := make(map[string][]ParsedHost)
	var sources []string
	sourceSet := make(map[string]bool)

	for _, host := range hosts {
		if !sourceSet[host.Source] {
			sources = append(sources, host.Source)
			sourceSet[host.Source] = true
		}
		grouped[host.Source] = append(grouped[host.Source], host)
	}

	// Write local hosts first
	if localHosts, ok := grouped["local"]; ok && len(localHosts) > 0 {
		for i, host := range localHosts {
			// Write comments
			for _, comment := range host.Comments {
				if _, err := fmt.Fprintln(w, comment); err != nil {
					return err
				}
			}

			// Write host lines
			for _, line := range host.Lines {
				if _, err := fmt.Fprintln(w, line); err != nil {
					return err
				}
			}

			// Add spacing between hosts
			if i < len(localHosts)-1 {
				if _, err := fmt.Fprintln(w); err != nil {
					return err
				}
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}

	// Write managed sections
	for _, source := range sources {
		if source == "local" {
			continue
		}

		hosts := grouped[source]
		if len(hosts) == 0 {
			continue
		}

		// Extract managed section name
		managedName := strings.TrimPrefix(source, "managed:")

		// Write section markers
		if _, err := fmt.Fprintf(w, "%s %s ===\n", markerPrefix, managedName); err != nil {
			return err
		}

		for i, host := range hosts {
			// Write host lines
			for _, line := range host.Lines {
				if _, err := fmt.Fprintln(w, line); err != nil {
					return err
				}
			}

			// Add spacing between hosts
			if i < len(hosts)-1 {
				if _, err := fmt.Fprintln(w); err != nil {
					return err
				}
			}
		}

		if _, err := fmt.Fprintf(w, "%s %s ===\n", markerSuffix, managedName); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}

	return nil
}

// writeLinearConfig writes config without grouping
func (p *Parser) writeLinearConfig(w io.Writer, hosts []ParsedHost) error {
	for i, host := range hosts {
		// Write comments
		for _, comment := range host.Comments {
			if _, err := fmt.Fprintln(w, comment); err != nil {
				return err
			}
		}

		// Write host lines
		for _, line := range host.Lines {
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
		}

		// Add spacing between hosts
		if i < len(hosts)-1 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}

	return nil
}

// ValidateHosts checks for duplicate host names
func ValidateHosts(hosts []Host) error {
	seen := make(map[string]string) // host name -> source

	for _, host := range hosts {
		if err := host.Validate(); err != nil {
			return fmt.Errorf("invalid host %s: %w", host.Name, err)
		}

		if source, exists := seen[host.Name]; exists {
			return fmt.Errorf("duplicate host name '%s' found in %s and %s",
				host.Name, source, host.Source)
		}
		seen[host.Name] = host.Source
	}

	return nil
}

// DeduplicateHosts removes duplicate hosts based on priority
func DeduplicateHosts(hosts []Host) []Host {
	// Map to track highest priority host for each name
	hostMap := make(map[string]Host)

	for _, host := range hosts {
		if existing, exists := hostMap[host.Name]; exists {
			// Keep the host with higher priority
			if host.Priority > existing.Priority {
				hostMap[host.Name] = host
			}
		} else {
			hostMap[host.Name] = host
		}
	}

	// Convert map back to slice
	result := make([]Host, 0, len(hostMap))
	for _, host := range hostMap {
		result = append(result, host)
	}

	return result
}

// SortHostsByPriority sorts hosts by priority (higher first)
func SortHostsByPriority(hosts []Host) {
	sort.Slice(hosts, func(i, j int) bool {
		if hosts[i].Priority == hosts[j].Priority {
			// Secondary sort by source name for consistency
			return hosts[i].Source < hosts[j].Source
		}
		return hosts[i].Priority > hosts[j].Priority
	})
}