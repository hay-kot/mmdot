package ssh

import (
	"bytes"
	"strings"
	"testing"
)

func TestParser_Parse(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		preserveLocal bool
		wantHosts     int
		checkHost     func(t *testing.T, hosts []ParsedHost)
	}{
		{
			name: "parse simple hosts",
			input: `Host server1
    Hostname 192.168.1.1
    User admin

Host server2
    Hostname 192.168.1.2
    User deploy`,
			preserveLocal: true,
			wantHosts:     2,
			checkHost: func(t *testing.T, hosts []ParsedHost) {
				if hosts[0].Name != "server1" {
					t.Errorf("First host name = %v, want server1", hosts[0].Name)
				}
				if hosts[1].Name != "server2" {
					t.Errorf("Second host name = %v, want server2", hosts[1].Name)
				}
				if hosts[0].Source != "local" {
					t.Errorf("First host source = %v, want local", hosts[0].Source)
				}
			},
		},
		{
			name: "parse hosts with comments",
			input: `# Production servers
Host prod
    Hostname prod.example.com
    # Important: use key auth
    IdentityFile ~/.ssh/prod_key`,
			preserveLocal: true,
			wantHosts:     1,
			checkHost: func(t *testing.T, hosts []ParsedHost) {
				if len(hosts[0].Comments) != 1 {
					t.Errorf("Comments count = %v, want 1", len(hosts[0].Comments))
				}
				if !strings.Contains(hosts[0].Comments[0], "Production servers") {
					t.Errorf("Comment doesn't contain expected text")
				}
			},
		},
		{
			name: "parse managed sections",
			input: `Host local-server
    Hostname localhost

# === BEGIN MMDOT MANAGED: personal ===
Host managed-server
    Hostname 192.168.1.100
    User admin
# === END MMDOT MANAGED: personal ===

Host another-local
    Hostname local.test`,
			preserveLocal: true,
			wantHosts:     2, // Only local hosts when preserving
			checkHost: func(t *testing.T, hosts []ParsedHost) {
				for _, host := range hosts {
					if host.Source != "local" {
						t.Errorf("Expected only local hosts when preserving, got %v", host.Source)
					}
				}
			},
		},
		{
			name: "parse managed sections without preserving",
			input: `# === BEGIN MMDOT MANAGED: work ===
Host work-server
    Hostname work.example.com
# === END MMDOT MANAGED: work ===`,
			preserveLocal: false,
			wantHosts:     1,
			checkHost: func(t *testing.T, hosts []ParsedHost) {
				if hosts[0].Source != "managed:work" {
					t.Errorf("Host source = %v, want managed:work", hosts[0].Source)
				}
			},
		},
		{
			name: "parse wildcard hosts",
			input: `Host *.example.com
    User admin
    ProxyJump bastion`,
			preserveLocal: true,
			wantHosts:     1,
			checkHost: func(t *testing.T, hosts []ParsedHost) {
				if hosts[0].Name != "*.example.com" {
					t.Errorf("Host name = %v, want *.example.com", hosts[0].Name)
				}
			},
		},
		{
			name:          "empty config",
			input:         "",
			preserveLocal: true,
			wantHosts:     0,
		},
		{
			name: "config with only comments",
			input: `# SSH Configuration
# Managed by mmdot
#
# Local hosts`,
			preserveLocal: true,
			wantHosts:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.preserveLocal)
			reader := strings.NewReader(tt.input)

			hosts, err := parser.Parse(reader)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if len(hosts) != tt.wantHosts {
				t.Errorf("Parse() returned %d hosts, want %d", len(hosts), tt.wantHosts)
			}

			if tt.checkHost != nil && len(hosts) > 0 {
				tt.checkHost(t, hosts)
			}
		})
	}
}

func TestParser_MergeHosts(t *testing.T) {
	parser := NewParser(true)

	existing := []ParsedHost{
		{
			Name:   "local-host",
			Lines:  []string{"Host local-host", "    Hostname local.test"},
			Source: "local",
		},
		{
			Name:   "old-managed",
			Lines:  []string{"Host old-managed", "    Hostname old.test"},
			Source: "managed:personal",
		},
	}

	managed := []Host{
		{
			Name:     "new-managed",
			Hostname: "new.test",
			User:     "admin",
		},
		{
			Name:     "another-managed",
			Hostname: "another.test",
			Port:     2222,
		},
	}

	result := parser.MergeHosts(existing, managed, "personal")

	// Should have local-host + 2 new managed hosts
	if len(result) != 3 {
		t.Errorf("MergeHosts returned %d hosts, want 3", len(result))
	}

	// Check that local host is preserved
	foundLocal := false
	for _, host := range result {
		if host.Name == "local-host" && host.Source == "local" {
			foundLocal = true
			break
		}
	}
	if !foundLocal {
		t.Error("Local host was not preserved")
	}

	// Check that old managed host is replaced
	for _, host := range result {
		if host.Name == "old-managed" {
			t.Error("Old managed host should have been removed")
		}
	}

	// Check new managed hosts are added
	foundNew := false
	foundAnother := false
	for _, host := range result {
		if host.Name == "new-managed" {
			foundNew = true
			if host.Source != "managed:personal" {
				t.Errorf("New managed host has wrong source: %v", host.Source)
			}
		}
		if host.Name == "another-managed" {
			foundAnother = true
		}
	}
	if !foundNew || !foundAnother {
		t.Error("Not all new managed hosts were added")
	}
}

func TestParser_WriteConfig(t *testing.T) {
	parser := NewParser(true)

	hosts := []ParsedHost{
		{
			Name:     "local-host",
			Lines:    []string{"Host local-host", "    Hostname local.test"},
			Comments: []string{"# Local development"},
			Source:   "local",
		},
		{
			Name:   "managed-host",
			Lines:  []string{"Host managed-host", "    Hostname managed.test"},
			Source: "managed:personal",
		},
	}

	t.Run("grouped config", func(t *testing.T) {
		var buf bytes.Buffer
		err := parser.WriteConfig(&buf, hosts, true)
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		output := buf.String()

		// Check for local host and comment
		if !strings.Contains(output, "# Local development") {
			t.Error("Comment not found in output")
		}
		if !strings.Contains(output, "Host local-host") {
			t.Error("Local host not found in output")
		}

		// Check for managed section markers
		if !strings.Contains(output, "# === BEGIN MMDOT MANAGED: personal ===") {
			t.Error("Managed section begin marker not found")
		}
		if !strings.Contains(output, "# === END MMDOT MANAGED: personal ===") {
			t.Error("Managed section end marker not found")
		}
		if !strings.Contains(output, "Host managed-host") {
			t.Error("Managed host not found in output")
		}
	})

	t.Run("linear config", func(t *testing.T) {
		var buf bytes.Buffer
		err := parser.WriteConfig(&buf, hosts, false)
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		output := buf.String()

		// Should not have section markers
		if strings.Contains(output, "# === BEGIN MMDOT MANAGED") {
			t.Error("Should not have section markers in linear mode")
		}

		// Should have both hosts
		if !strings.Contains(output, "Host local-host") {
			t.Error("Local host not found in output")
		}
		if !strings.Contains(output, "Host managed-host") {
			t.Error("Managed host not found in output")
		}
	})
}

func TestValidateHosts(t *testing.T) {
	tests := []struct {
		name    string
		hosts   []Host
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid hosts",
			hosts: []Host{
				{Name: "server1", Hostname: "server1.example.com", Source: "file1"},
				{Name: "server2", Hostname: "server2.example.com", Source: "file2"},
			},
			wantErr: false,
		},
		{
			name: "duplicate hosts",
			hosts: []Host{
				{Name: "server", Hostname: "server1.example.com", Source: "file1"},
				{Name: "server", Hostname: "server2.example.com", Source: "file2"},
			},
			wantErr: true,
			errMsg:  "duplicate host name",
		},
		{
			name: "invalid host",
			hosts: []Host{
				{Name: "", Hostname: "server.example.com"},
			},
			wantErr: true,
			errMsg:  "host name cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHosts(tt.hosts)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHosts() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateHosts() error = %v, want to contain %v", err, tt.errMsg)
			}
		})
	}
}

func TestDeduplicateHosts(t *testing.T) {
	hosts := []Host{
		{Name: "server", Hostname: "server1.example.com", Priority: 10, Source: "personal"},
		{Name: "server", Hostname: "server2.example.com", Priority: 20, Source: "work"},
		{Name: "unique", Hostname: "unique.example.com", Priority: 15, Source: "personal"},
		{Name: "another", Hostname: "another1.example.com", Priority: 5, Source: "public"},
		{Name: "another", Hostname: "another2.example.com", Priority: 3, Source: "test"},
	}

	result := DeduplicateHosts(hosts)

	// Should have 3 unique hosts
	if len(result) != 3 {
		t.Errorf("DeduplicateHosts returned %d hosts, want 3", len(result))
	}

	// Check that higher priority hosts are kept
	for _, host := range result {
		if host.Name == "server" && host.Hostname != "server2.example.com" {
			t.Error("Should keep server with higher priority (work)")
		}
		if host.Name == "another" && host.Hostname != "another1.example.com" {
			t.Error("Should keep another with higher priority (public)")
		}
	}

	// Check unique host is preserved
	foundUnique := false
	for _, host := range result {
		if host.Name == "unique" {
			foundUnique = true
			break
		}
	}
	if !foundUnique {
		t.Error("Unique host should be preserved")
	}
}