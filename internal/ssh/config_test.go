package ssh

import (
	"strings"
	"testing"
)

func TestHost_String(t *testing.T) {
	tests := []struct {
		name     string
		host     Host
		expected []string // Lines that should be in the output
	}{
		{
			name: "basic host",
			host: Host{
				Name:     "test-server",
				Hostname: "192.168.1.100",
				User:     "admin",
				Port:     2222,
			},
			expected: []string{
				"Host test-server",
				"    Hostname 192.168.1.100",
				"    User admin",
				"    Port 2222",
			},
		},
		{
			name: "host with identity and proxy",
			host: Host{
				Name:         "prod-server",
				Hostname:     "prod.internal",
				User:         "deploy",
				IdentityFile: "~/.ssh/prod_key",
				ProxyJump:    "bastion.example.com",
			},
			expected: []string{
				"Host prod-server",
				"    Hostname prod.internal",
				"    User deploy",
				"    IdentityFile ~/.ssh/prod_key",
				"    ProxyJump bastion.example.com",
			},
		},
		{
			name: "host with forwarding options",
			host: Host{
				Name:         "dev-vm",
				Hostname:     "localhost",
				Port:         2222,
				ForwardAgent: boolPtr(true),
				ForwardX11:   boolPtr(false),
				LocalForward: []string{
					"8080:localhost:80",
					"3000:localhost:3000",
				},
			},
			expected: []string{
				"Host dev-vm",
				"    Hostname localhost",
				"    Port 2222",
				"    ForwardAgent yes",
				"    ForwardX11 no",
				"    LocalForward 8080 localhost:80",
				"    LocalForward 3000 localhost:3000",
			},
		},
		{
			name: "host with custom options",
			host: Host{
				Name:     "custom-server",
				Hostname: "custom.example.com",
				Custom: []string{
					"ServerAliveInterval 60",
					"ServerAliveCountMax 3",
					"StrictHostKeyChecking ask",
				},
			},
			expected: []string{
				"Host custom-server",
				"    Hostname custom.example.com",
				"    ServerAliveInterval 60",
				"    ServerAliveCountMax 3",
				"    StrictHostKeyChecking ask",
			},
		},
		{
			name: "minimal host",
			host: Host{
				Name:     "minimal",
				Hostname: "min.example.com",
			},
			expected: []string{
				"Host minimal",
				"    Hostname min.example.com",
			},
		},
		{
			name: "wildcard host",
			host: Host{
				Name:     "*.example.com",
				Hostname: "%h.internal",
				User:     "admin",
			},
			expected: []string{
				"Host *.example.com",
				"    Hostname %h.internal",
				"    User admin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.host.String()

			// Check that all expected lines are present
			for _, expected := range tt.expected {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected output to contain '%s', but it didn't.\nGot:\n%s", expected, result)
				}
			}

			// Check that output doesn't end with newline (as per implementation)
			if strings.HasSuffix(result, "\n") {
				t.Errorf("Output should not end with newline, but it does:\n%s", result)
			}

			// Check that Host line is first
			if !strings.HasPrefix(result, "Host ") {
				t.Errorf("Output should start with 'Host ', but starts with: %s", strings.Split(result, "\n")[0])
			}
		})
	}
}

func TestHost_Validate(t *testing.T) {
	tests := []struct {
		name    string
		host    Host
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid host",
			host: Host{
				Name:     "valid-host",
				Hostname: "example.com",
				Port:     22,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			host: Host{
				Hostname: "example.com",
			},
			wantErr: true,
			errMsg:  "host name cannot be empty",
		},
		{
			name: "missing hostname",
			host: Host{
				Name: "test",
			},
			wantErr: true,
			errMsg:  "hostname cannot be empty",
		},
		{
			name: "invalid port - negative",
			host: Host{
				Name:     "test",
				Hostname: "example.com",
				Port:     -1,
			},
			wantErr: true,
			errMsg:  "invalid port",
		},
		{
			name: "invalid port - too high",
			host: Host{
				Name:     "test",
				Hostname: "example.com",
				Port:     65536,
			},
			wantErr: true,
			errMsg:  "invalid port",
		},
		{
			name: "port zero is valid",
			host: Host{
				Name:     "test",
				Hostname: "example.com",
				Port:     0, // 0 means default (22)
			},
			wantErr: false,
		},
		{
			name: "wildcard host is valid",
			host: Host{
				Name:     "*.example.com",
				Hostname: "%h.internal",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.host.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Validate() error message = %v, want to contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestHostSource_HasEncryption(t *testing.T) {
	tests := []struct {
		name   string
		source HostSource
		want   bool
	}{
		{
			name: "has encryption",
			source: HostSource{
				EncryptedFile: "test.age",
				Recipients:    []string{"age1xxx"},
			},
			want: true,
		},
		{
			name: "no recipients",
			source: HostSource{
				EncryptedFile: "test.age",
			},
			want: false,
		},
		{
			name: "no encrypted file",
			source: HostSource{
				Recipients: []string{"age1xxx"},
			},
			want: false,
		},
		{
			name:   "empty source",
			source: HostSource{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.source.HasEncryption(); got != tt.want {
				t.Errorf("HasEncryption() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHostSource_NeedsDecryption(t *testing.T) {
	tests := []struct {
		name   string
		source HostSource
		want   bool
	}{
		{
			name: "needs decryption",
			source: HostSource{
				EncryptedFile: "test.age",
			},
			want: true,
		},
		{
			name: "inline hosts don't need decryption",
			source: HostSource{
				Hosts: []Host{{Name: "test", Hostname: "example.com"}},
			},
			want: false,
		},
		{
			name:   "empty source",
			source: HostSource{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.source.NeedsDecryption(); got != tt.want {
				t.Errorf("NeedsDecryption() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLocalForwardParsing(t *testing.T) {
	host := Host{
		Name:     "test",
		Hostname: "example.com",
		LocalForward: []string{
			"8080:localhost:80",
			"3000:localhost:3000",
			"invalid-forward", // Should be ignored
		},
		RemoteForward: []string{
			"9090:localhost:90",
			"also:invalid", // Should have exactly one colon to be valid
		},
	}

	result := host.String()

	// Valid forwards should be included
	if !strings.Contains(result, "LocalForward 8080 localhost:80") {
		t.Error("Expected valid LocalForward to be included")
	}
	if !strings.Contains(result, "LocalForward 3000 localhost:3000") {
		t.Error("Expected valid LocalForward to be included")
	}
	if !strings.Contains(result, "RemoteForward 9090 localhost:90") {
		t.Error("Expected valid RemoteForward to be included")
	}

	// Invalid forwards should not appear
	if strings.Contains(result, "invalid-forward") {
		t.Error("Invalid forward should not be included")
	}
}

// Helper function to create bool pointers
func boolPtr(b bool) *bool {
	return &b
}

