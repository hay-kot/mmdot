package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/hay-kot/mmdot/internal/appdata"
)

func TestAppDataConfig_YAMLParsing(t *testing.T) {
	input := `
appdata:
  storage: ~/.local/share/appdata
  apps:
    - vscode
    - iterm2
  ignore:
    - slack
  custom:
    - id: myapp
      name: My App
      files:
        - ~/.myapp/config.yml
      xdg_files:
        - myapp/settings.json
  snapshot_retention: 5
`

	var cfg ConfigFile
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if cfg.AppData.Storage != "~/.local/share/appdata" {
		t.Errorf("Storage = %q, want %q", cfg.AppData.Storage, "~/.local/share/appdata")
	}

	if len(cfg.AppData.Apps) != 2 {
		t.Fatalf("Apps length = %d, want 2", len(cfg.AppData.Apps))
	}
	if cfg.AppData.Apps[0] != "vscode" {
		t.Errorf("Apps[0] = %q, want %q", cfg.AppData.Apps[0], "vscode")
	}
	if cfg.AppData.Apps[1] != "iterm2" {
		t.Errorf("Apps[1] = %q, want %q", cfg.AppData.Apps[1], "iterm2")
	}

	if len(cfg.AppData.Ignore) != 1 || cfg.AppData.Ignore[0] != "slack" {
		t.Errorf("Ignore = %v, want [slack]", cfg.AppData.Ignore)
	}

	if len(cfg.AppData.Custom) != 1 {
		t.Fatalf("Custom length = %d, want 1", len(cfg.AppData.Custom))
	}

	want := appdata.CustomEntry{
		ID:       "myapp",
		Name:     "My App",
		Files:    []string{"~/.myapp/config.yml"},
		XDGFiles: []string{"myapp/settings.json"},
	}

	got := cfg.AppData.Custom[0]
	if got.ID != want.ID {
		t.Errorf("Custom[0].ID = %q, want %q", got.ID, want.ID)
	}
	if got.Name != want.Name {
		t.Errorf("Custom[0].Name = %q, want %q", got.Name, want.Name)
	}
	if len(got.Files) != 1 || got.Files[0] != want.Files[0] {
		t.Errorf("Custom[0].Files = %v, want %v", got.Files, want.Files)
	}
	if len(got.XDGFiles) != 1 || got.XDGFiles[0] != want.XDGFiles[0] {
		t.Errorf("Custom[0].XDGFiles = %v, want %v", got.XDGFiles, want.XDGFiles)
	}

	if cfg.AppData.SnapshotRetention != 5 {
		t.Errorf("SnapshotRetention = %d, want 5", cfg.AppData.SnapshotRetention)
	}
}

func TestAppDataConfig_StorageResolution(t *testing.T) {
	tests := []struct {
		name      string
		storage   string
		configDir string
		want      string
		usesHome  bool
	}{
		{
			name:      "relative path",
			storage:   "data/backups",
			configDir: "/config/dir",
			want:      "/config/dir/data/backups",
		},
		{
			name:      "tilde path",
			storage:   "~/.local/share/appdata",
			configDir: "/config/dir",
			usesHome:  true,
			want:      ".local/share/appdata", // joined with home dir in assertion
		},
		{
			name:      "empty storage skipped",
			storage:   "",
			configDir: "/config/dir",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ConfigFile{
				AppData: AppDataConfig{
					Storage: tt.storage,
				},
			}

			pr := PathResolver{configDir: tt.configDir}
			if err := cfg.resolvePaths(pr); err != nil {
				t.Fatalf("resolvePaths() error: %v", err)
			}

			if tt.usesHome {
				homeDir, _ := os.UserHomeDir()
				expected := filepath.Join(homeDir, tt.want)
				if cfg.AppData.Storage != expected {
					t.Errorf("Storage = %q, want %q", cfg.AppData.Storage, expected)
				}
			} else {
				if cfg.AppData.Storage != tt.want {
					t.Errorf("Storage = %q, want %q", cfg.AppData.Storage, tt.want)
				}
			}
		})
	}
}
