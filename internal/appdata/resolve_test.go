package appdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestXdgConfigDir_WithEnv(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	got := xdgConfigDir()
	if got != "/custom/config" {
		t.Errorf("xdgConfigDir() = %q, want /custom/config", got)
	}
}

func TestXdgConfigDir_Fallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config")
	got := xdgConfigDir()
	if got != want {
		t.Errorf("xdgConfigDir() = %q, want %q", got, want)
	}
}

func TestResolveApp(t *testing.T) {
	home, _ := os.UserHomeDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	def := AppDef{
		Name:     "Git",
		Files:    []string{".gitconfig"},
		XDGFiles: []string{"git/config"},
	}

	storageDir := "/tmp/storage"
	resolved := ResolveApp("git", def, storageDir)

	if resolved.ID != "git" {
		t.Errorf("ID = %q, want git", resolved.ID)
	}
	if resolved.Name != "Git" {
		t.Errorf("Name = %q, want Git", resolved.Name)
	}
	if len(resolved.Entries) != 2 {
		t.Fatalf("len(Entries) = %d, want 2", len(resolved.Entries))
	}

	// Check home file entry
	e0 := resolved.Entries[0]
	wantHome := filepath.Join(home, ".gitconfig")
	wantStorage := filepath.Join(storageDir, "git", ".gitconfig")
	if e0.HomePath != wantHome {
		t.Errorf("Entries[0].HomePath = %q, want %q", e0.HomePath, wantHome)
	}
	if e0.StoragePath != wantStorage {
		t.Errorf("Entries[0].StoragePath = %q, want %q", e0.StoragePath, wantStorage)
	}

	// Check XDG file entry
	e1 := resolved.Entries[1]
	wantHome = filepath.Join(home, ".config", "git/config")
	wantStorage = filepath.Join(storageDir, "git", ".config", "git/config")
	if e1.HomePath != wantHome {
		t.Errorf("Entries[1].HomePath = %q, want %q", e1.HomePath, wantHome)
	}
	if e1.StoragePath != wantStorage {
		t.Errorf("Entries[1].StoragePath = %q, want %q", e1.StoragePath, wantStorage)
	}
}

func TestResolveApp_NoFiles(t *testing.T) {
	def := AppDef{Name: "Empty"}
	resolved := ResolveApp("empty", def, "/tmp/storage")

	if len(resolved.Entries) != 0 {
		t.Errorf("len(Entries) = %d, want 0", len(resolved.Entries))
	}
}
