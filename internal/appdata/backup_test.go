package appdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupApp(t *testing.T) {
	tmp := t.TempDir()

	homeDir := filepath.Join(tmp, "home")
	storageDir := filepath.Join(tmp, "storage")

	// Create source files in "home"
	src := filepath.Join(homeDir, ".bashrc")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("export PATH=/usr/bin"), 0644); err != nil {
		t.Fatal(err)
	}

	app := ResolvedApp{
		ID:   "bash",
		Name: "Bash",
		Entries: []FileEntry{
			{HomePath: src, StoragePath: filepath.Join(storageDir, "bash", ".bashrc")},
		},
	}

	result := backupApp(app, false)
	if result.Err != nil {
		t.Fatalf("backupApp() error: %v", result.Err)
	}
	if result.Copied != 1 {
		t.Errorf("copied = %d, want 1", result.Copied)
	}
	if result.Skipped != 0 {
		t.Errorf("skipped = %d, want 0", result.Skipped)
	}

	got, err := os.ReadFile(filepath.Join(storageDir, "bash", ".bashrc"))
	if err != nil {
		t.Fatalf("reading backed up file: %v", err)
	}
	if string(got) != "export PATH=/usr/bin" {
		t.Errorf("content = %q, want %q", got, "export PATH=/usr/bin")
	}
}

func TestBackupApp_DryRun(t *testing.T) {
	tmp := t.TempDir()

	src := filepath.Join(tmp, "home", ".bashrc")
	if err := os.MkdirAll(filepath.Dir(src), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(tmp, "storage", "bash", ".bashrc")

	app := ResolvedApp{
		ID:   "bash",
		Name: "Bash",
		Entries: []FileEntry{
			{HomePath: src, StoragePath: dst},
		},
	}

	result := backupApp(app, true)
	if result.Err != nil {
		t.Fatalf("backupApp() error: %v", result.Err)
	}
	if result.Copied != 1 {
		t.Errorf("copied = %d, want 1", result.Copied)
	}

	// File should not exist in storage
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("dry run should not create files")
	}
}

func TestBackupApp_MissingSource(t *testing.T) {
	tmp := t.TempDir()

	app := ResolvedApp{
		ID:   "missing",
		Name: "Missing",
		Entries: []FileEntry{
			{
				HomePath:    filepath.Join(tmp, "does-not-exist"),
				StoragePath: filepath.Join(tmp, "storage", "missing", "file"),
			},
		},
	}

	result := backupApp(app, false)
	if result.Err != nil {
		t.Fatalf("backupApp() error: %v", result.Err)
	}
	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", result.Skipped)
	}
	if result.Copied != 0 {
		t.Errorf("copied = %d, want 0", result.Copied)
	}
}

func TestBackupApp_StatError(t *testing.T) {
	tmp := t.TempDir()

	// Create a file inside a directory with no read permission
	srcDir := filepath.Join(tmp, "noperm")
	src := filepath.Join(srcDir, "config")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	// Remove execute permission so stat on the file fails with EACCES, not ENOENT
	if err := os.Chmod(srcDir, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(srcDir, 0755) })

	app := ResolvedApp{
		ID:   "noperm",
		Name: "NoPerm",
		Entries: []FileEntry{
			{HomePath: src, StoragePath: filepath.Join(tmp, "storage", "noperm", "config")},
		},
	}

	result := backupApp(app, false)
	if result.Err == nil {
		t.Fatal("expected error for permission-denied stat, got nil")
	}
}

func TestBackupAll(t *testing.T) {
	tmp := t.TempDir()

	// Create two apps with home files
	var apps []ResolvedApp
	for _, name := range []string{"app1", "app2", "app3"} {
		src := filepath.Join(tmp, "home", name, "config")
		if err := os.MkdirAll(filepath.Dir(src), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(src, []byte(name+"-data"), 0644); err != nil {
			t.Fatal(err)
		}

		apps = append(apps, ResolvedApp{
			ID:   name,
			Name: name,
			Entries: []FileEntry{
				{HomePath: src, StoragePath: filepath.Join(tmp, "storage", name, "config")},
			},
		})
	}

	results := BackupAll(apps, false)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	for _, r := range results {
		if r.Err != nil {
			t.Errorf("app %s error: %v", r.App.ID, r.Err)
		}
		if r.Copied != 1 {
			t.Errorf("app %s copied = %d, want 1", r.App.ID, r.Copied)
		}
	}

	// Verify all files exist in storage
	for _, name := range []string{"app1", "app2", "app3"} {
		got, err := os.ReadFile(filepath.Join(tmp, "storage", name, "config"))
		if err != nil {
			t.Errorf("reading %s: %v", name, err)
			continue
		}
		want := name + "-data"
		if string(got) != want {
			t.Errorf("%s content = %q, want %q", name, got, want)
		}
	}
}
