package core

import (
	"os"
	"path/filepath"
	"strings"
)

// PathResolver provides a resolving service for paths that turns a relative or
// paths with '~' type symbols into absolute paths.
type PathResolver struct {
	configDir string // config directory used to set relative path roots
}

func (pr PathResolver) Resolve(ip string) (string, error) {
	// Handle home directory expansion
	if strings.HasPrefix(ip, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		ip = filepath.Join(homeDir, strings.TrimPrefix(ip, "~"))
	}

	// If already absolute, return as-is
	if filepath.IsAbs(ip) {
		return filepath.Clean(ip), nil
	}

	// Resolve relative to config directory
	if pr.configDir != "" {
		return filepath.Join(pr.configDir, ip), nil
	}

	// Fallback to absolute path from current directory
	absPath, err := filepath.Abs(ip)
	if err != nil {
		return "", err
	}

	return absPath, nil
}
