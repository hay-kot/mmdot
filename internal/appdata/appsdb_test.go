package appdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertedAppsYAML(t *testing.T) {
	db, err := LoadAppDB()
	if err != nil {
		t.Fatalf("LoadAppDB() error: %v", err)
	}

	// Count .cfg files in source directory to compare
	srcDir := filepath.Join("..", "..", "mackup", "src", "mackup", "applications")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("reading source dir: %v", err)
	}

	var cfgCount int
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".cfg") {
			cfgCount++
		}
	}

	if len(db) != cfgCount {
		t.Errorf("app count = %d, want %d (.cfg files)", len(db), cfgCount)
	}

	// Spot-check: git
	git, ok := db["git"]
	if !ok {
		t.Fatal("missing entry: git")
	}
	if git.Name != "Git" {
		t.Errorf("git.Name = %q, want %q", git.Name, "Git")
	}
	if len(git.Files) != 1 || git.Files[0] != ".gitconfig" {
		t.Errorf("git.Files = %v, want [.gitconfig]", git.Files)
	}
	if len(git.XDGFiles) != 3 {
		t.Errorf("git.XDGFiles count = %d, want 3", len(git.XDGFiles))
	}

	// Spot-check: zsh
	zsh, ok := db["zsh"]
	if !ok {
		t.Fatal("missing entry: zsh")
	}
	if zsh.Name != "Zsh" {
		t.Errorf("zsh.Name = %q, want %q", zsh.Name, "Zsh")
	}
	if len(zsh.Files) != 5 {
		t.Errorf("zsh.Files count = %d, want 5", len(zsh.Files))
	}
	if len(zsh.XDGFiles) != 0 {
		t.Errorf("zsh.XDGFiles count = %d, want 0", len(zsh.XDGFiles))
	}

	// Spot-check: neovim
	nvim, ok := db["neovim"]
	if !ok {
		t.Fatal("missing entry: neovim")
	}
	if nvim.Name != "neovim" {
		t.Errorf("neovim.Name = %q, want %q", nvim.Name, "neovim")
	}
	if len(nvim.Files) != 2 {
		t.Errorf("neovim.Files count = %d, want 2", len(nvim.Files))
	}
	if len(nvim.XDGFiles) != 10 {
		t.Errorf("neovim.XDGFiles count = %d, want 10", len(nvim.XDGFiles))
	}
}
