package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathResolver_Resolve(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	tests := []struct {
		name      string
		configDir string
		input     string
		want      string
		wantErr   bool
	}{
		{
			name:      "absolute path",
			configDir: "/config/dir",
			input:     "/absolute/path",
			want:      "/absolute/path",
			wantErr:   false,
		},
		{
			name:      "home directory expansion",
			configDir: "/config/dir",
			input:     "~/Documents",
			want:      filepath.Join(homeDir, "Documents"),
			wantErr:   false,
		},
		{
			name:      "home directory only",
			configDir: "/config/dir",
			input:     "~",
			want:      homeDir,
			wantErr:   false,
		},
		{
			name:      "relative path with config dir",
			configDir: "/config/dir",
			input:     "relative/path",
			want:      "/config/dir/relative/path",
			wantErr:   false,
		},
		{
			name:      "relative path without config dir",
			configDir: "",
			input:     "relative/path",
			want:      func() string {
				cwd, _ := os.Getwd()
				return filepath.Join(cwd, "relative/path")
			}(),
			wantErr: false,
		},
		{
			name:      "dot path with config dir",
			configDir: "/config/dir",
			input:     "./file.txt",
			want:      "/config/dir/file.txt",
			wantErr:   false,
		},
		{
			name:      "parent directory with config dir",
			configDir: "/config/dir",
			input:     "../file.txt",
			want:      "/config/file.txt",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := PathResolver{
				configDir: tt.configDir,
			}
			got, err := pr.Resolve(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("PathResolver.Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("PathResolver.Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathResolver_Resolve_CleansPaths(t *testing.T) {
	pr := PathResolver{
		configDir: "/config/dir",
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "absolute path with double slashes",
			input: "/absolute//path",
			want:  "/absolute/path",
		},
		{
			name:  "absolute path with trailing slash",
			input: "/absolute/path/",
			want:  "/absolute/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pr.Resolve(tt.input)
			if err != nil {
				t.Errorf("PathResolver.Resolve() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("PathResolver.Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}
