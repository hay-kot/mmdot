package appdata

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestCreateSnapshot(t *testing.T) {
	storageDir := t.TempDir()

	// Create some files in storage
	if err := os.MkdirAll(filepath.Join(storageDir, "bash"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "bash", ".bashrc"), []byte("bashrc"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "toplevel.txt"), []byte("top"), 0644); err != nil {
		t.Fatal(err)
	}

	snapPath, err := CreateSnapshot(storageDir)
	if err != nil {
		t.Fatalf("CreateSnapshot() error: %v", err)
	}

	if !strings.HasSuffix(snapPath, ".zip") {
		t.Errorf("snapshot path %q does not end with .zip", snapPath)
	}

	// Verify zip contents
	r, err := zip.OpenReader(snapPath)
	if err != nil {
		t.Fatalf("opening zip: %v", err)
	}
	defer func() { _ = r.Close() }()

	names := make(map[string]bool)
	for _, f := range r.File {
		names[f.Name] = true
	}

	// Should contain our files
	if !names["bash/"] {
		t.Error("zip missing bash/ directory")
	}
	if !names["bash/.bashrc"] {
		t.Error("zip missing bash/.bashrc")
	}
	if !names["toplevel.txt"] {
		t.Error("zip missing toplevel.txt")
	}

	// Should NOT contain .snapshots
	for name := range names {
		if strings.HasPrefix(name, snapshotDir) {
			t.Errorf("zip should not contain snapshot dir, found: %s", name)
		}
	}
}

func TestPruneSnapshots(t *testing.T) {
	storageDir := t.TempDir()
	snapDir := filepath.Join(storageDir, snapshotDir)

	if err := os.MkdirAll(snapDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create 5 fake snapshots with ordered names
	snapNames := []string{
		"2026-01-01T00-00-00.zip",
		"2026-01-02T00-00-00.zip",
		"2026-01-03T00-00-00.zip",
		"2026-01-04T00-00-00.zip",
		"2026-01-05T00-00-00.zip",
	}

	for _, name := range snapNames {
		if err := os.WriteFile(filepath.Join(snapDir, name), []byte("zip"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Keep 2 most recent
	if err := PruneSnapshots(storageDir, 2); err != nil {
		t.Fatalf("PruneSnapshots() error: %v", err)
	}

	entries, err := os.ReadDir(snapDir)
	if err != nil {
		t.Fatal(err)
	}

	var remaining []string
	for _, e := range entries {
		remaining = append(remaining, e.Name())
	}
	sort.Strings(remaining)

	if len(remaining) != 2 {
		t.Fatalf("got %d snapshots, want 2: %v", len(remaining), remaining)
	}

	// Should keep the two most recent
	if remaining[0] != "2026-01-04T00-00-00.zip" {
		t.Errorf("remaining[0] = %s, want 2026-01-04T00-00-00.zip", remaining[0])
	}
	if remaining[1] != "2026-01-05T00-00-00.zip" {
		t.Errorf("remaining[1] = %s, want 2026-01-05T00-00-00.zip", remaining[1])
	}
}

func TestPruneSnapshots_NoDir(t *testing.T) {
	// Should not error when snapshot dir doesn't exist
	if err := PruneSnapshots(t.TempDir(), 3); err != nil {
		t.Fatalf("PruneSnapshots() error: %v", err)
	}
}

func TestPruneSnapshots_FewerThanKeep(t *testing.T) {
	storageDir := t.TempDir()
	snapDir := filepath.Join(storageDir, snapshotDir)

	if err := os.MkdirAll(snapDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(snapDir, "2026-01-01T00-00-00.zip"), []byte("zip"), 0644); err != nil {
		t.Fatal(err)
	}

	// Keep 5 but only 1 exists - should be a no-op
	if err := PruneSnapshots(storageDir, 5); err != nil {
		t.Fatalf("PruneSnapshots() error: %v", err)
	}

	entries, err := os.ReadDir(snapDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1", len(entries))
	}
}
