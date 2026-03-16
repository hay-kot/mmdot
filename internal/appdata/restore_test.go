package appdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreApp(t *testing.T) {
	tmp := t.TempDir()

	storageFile := filepath.Join(tmp, "storage", "bash", ".bashrc")
	if err := os.MkdirAll(filepath.Dir(storageFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storageFile, []byte("export PATH=/usr/bin"), 0644); err != nil {
		t.Fatal(err)
	}

	homeFile := filepath.Join(tmp, "home", ".bashrc")

	app := ResolvedApp{
		ID:   "bash",
		Name: "Bash",
		Entries: []FileEntry{
			{HomePath: homeFile, StoragePath: storageFile},
		},
	}

	result := restoreApp(app, false)
	if result.Err != nil {
		t.Fatalf("restoreApp() error: %v", result.Err)
	}
	if result.Copied != 1 {
		t.Errorf("copied = %d, want 1", result.Copied)
	}
	if result.Skipped != 0 {
		t.Errorf("skipped = %d, want 0", result.Skipped)
	}

	got, err := os.ReadFile(homeFile)
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(got) != "export PATH=/usr/bin" {
		t.Errorf("content = %q, want %q", got, "export PATH=/usr/bin")
	}
}

func TestRestoreApp_DryRun(t *testing.T) {
	tmp := t.TempDir()

	storageFile := filepath.Join(tmp, "storage", "bash", ".bashrc")
	if err := os.MkdirAll(filepath.Dir(storageFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storageFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	homeFile := filepath.Join(tmp, "home", ".bashrc")

	app := ResolvedApp{
		ID:   "bash",
		Name: "Bash",
		Entries: []FileEntry{
			{HomePath: homeFile, StoragePath: storageFile},
		},
	}

	result := restoreApp(app, true)
	if result.Err != nil {
		t.Fatalf("restoreApp() error: %v", result.Err)
	}
	if result.Copied != 1 {
		t.Errorf("copied = %d, want 1", result.Copied)
	}

	if _, err := os.Stat(homeFile); !os.IsNotExist(err) {
		t.Error("dry run should not create files")
	}
}

func TestRestoreApp_MissingStorage(t *testing.T) {
	tmp := t.TempDir()

	app := ResolvedApp{
		ID:   "missing",
		Name: "Missing",
		Entries: []FileEntry{
			{
				HomePath:    filepath.Join(tmp, "home", "file"),
				StoragePath: filepath.Join(tmp, "storage", "does-not-exist"),
			},
		},
	}

	result := restoreApp(app, false)
	if result.Err != nil {
		t.Fatalf("restoreApp() error: %v", result.Err)
	}
	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", result.Skipped)
	}
	if result.Copied != 0 {
		t.Errorf("copied = %d, want 0", result.Copied)
	}
}

func TestRestoreAll(t *testing.T) {
	tmp := t.TempDir()

	var apps []ResolvedApp
	for _, name := range []string{"app1", "app2", "app3"} {
		storageFile := filepath.Join(tmp, "storage", name, "config")
		if err := os.MkdirAll(filepath.Dir(storageFile), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(storageFile, []byte(name+"-data"), 0644); err != nil {
			t.Fatal(err)
		}

		apps = append(apps, ResolvedApp{
			ID:   name,
			Name: name,
			Entries: []FileEntry{
				{HomePath: filepath.Join(tmp, "home", name, "config"), StoragePath: storageFile},
			},
		})
	}

	results := RestoreAll(apps, false)
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

	for _, name := range []string{"app1", "app2", "app3"} {
		got, err := os.ReadFile(filepath.Join(tmp, "home", name, "config"))
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
