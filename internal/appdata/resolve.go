package appdata

import (
	"os"
	"path/filepath"
	"strings"
)

// FileEntry represents a single file to back up or restore.
type FileEntry struct {
	// HomePath is the absolute path in the user's home directory.
	HomePath string
	// StoragePath is the absolute path in the storage directory.
	StoragePath string
}

// ResolvedApp contains the resolved file entries for an app.
type ResolvedApp struct {
	ID      string
	Name    string
	Tags    []string
	Entries []FileEntry
}

// xdgConfigDir returns $XDG_CONFIG_HOME or ~/.config as fallback.
func xdgConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

// expandHomePath resolves a file path that may use ~/ prefix, be absolute,
// or be relative to the home directory (the convention for embedded DB entries).
func expandHomePath(home, f string) string {
	if strings.HasPrefix(f, "~/") {
		return filepath.Join(home, f[2:])
	}
	if filepath.IsAbs(f) {
		return filepath.Clean(f)
	}
	return filepath.Join(home, f)
}

// ResolveApp converts a TaggedApp into a ResolvedApp with absolute paths.
// storageDir is the base storage directory. Each app gets a subdirectory.
func ResolveApp(id string, app TaggedApp, storageDir string) ResolvedApp {
	home, _ := os.UserHomeDir()
	appStorage := filepath.Join(storageDir, id)

	var entries []FileEntry

	for _, f := range app.Files {
		entries = append(entries, FileEntry{
			HomePath:    expandHomePath(home, f),
			StoragePath: filepath.Join(appStorage, f),
		})
	}

	xdgDir := xdgConfigDir()
	for _, f := range app.XDGFiles {
		entries = append(entries, FileEntry{
			HomePath:    filepath.Join(xdgDir, f),
			StoragePath: filepath.Join(appStorage, ".config", f),
		})
	}

	return ResolvedApp{
		ID:      id,
		Name:    app.Name,
		Tags:    app.Tags,
		Entries: entries,
	}
}
