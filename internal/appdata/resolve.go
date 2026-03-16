package appdata

import (
	"os"
	"path/filepath"
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

// ResolveApp converts an AppDef into a ResolvedApp with absolute paths.
// storageDir is the base storage directory. Each app gets a subdirectory.
func ResolveApp(id string, def AppDef, storageDir string) ResolvedApp {
	home, _ := os.UserHomeDir()
	appStorage := filepath.Join(storageDir, id)

	var entries []FileEntry

	for _, f := range def.Files {
		entries = append(entries, FileEntry{
			HomePath:    filepath.Join(home, f),
			StoragePath: filepath.Join(appStorage, f),
		})
	}

	xdgDir := xdgConfigDir()
	for _, f := range def.XDGFiles {
		entries = append(entries, FileEntry{
			HomePath:    filepath.Join(xdgDir, f),
			StoragePath: filepath.Join(appStorage, ".config", f),
		})
	}

	return ResolvedApp{
		ID:      id,
		Name:    def.Name,
		Entries: entries,
	}
}
