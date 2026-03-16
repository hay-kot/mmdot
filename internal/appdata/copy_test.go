package appdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyPath_File(t *testing.T) {
	tmp := t.TempDir()

	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "sub", "dst.txt")

	content := []byte("hello world")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyPath(src, dst); err != nil {
		t.Fatalf("CopyPath() error: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading dst: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}

	info, _ := os.Stat(dst)
	if info.Mode().Perm() != 0644 {
		t.Errorf("perm = %o, want 0644", info.Mode().Perm())
	}
}

func TestCopyPath_FilePermissions(t *testing.T) {
	tmp := t.TempDir()

	src := filepath.Join(tmp, "exec.sh")
	dst := filepath.Join(tmp, "copy.sh")

	if err := os.WriteFile(src, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := CopyPath(src, dst); err != nil {
		t.Fatalf("CopyPath() error: %v", err)
	}

	info, _ := os.Stat(dst)
	if info.Mode().Perm() != 0755 {
		t.Errorf("perm = %o, want 0755", info.Mode().Perm())
	}
}

func TestCopyPath_Directory(t *testing.T) {
	tmp := t.TempDir()

	srcDir := filepath.Join(tmp, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("aaa"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "b.txt"), []byte("bbb"), 0644); err != nil {
		t.Fatal(err)
	}

	dstDir := filepath.Join(tmp, "dst")
	if err := CopyPath(srcDir, dstDir); err != nil {
		t.Fatalf("CopyPath() error: %v", err)
	}

	// Verify files were copied
	gotA, err := os.ReadFile(filepath.Join(dstDir, "a.txt"))
	if err != nil {
		t.Fatalf("reading a.txt: %v", err)
	}
	if string(gotA) != "aaa" {
		t.Errorf("a.txt = %q, want %q", gotA, "aaa")
	}

	gotB, err := os.ReadFile(filepath.Join(dstDir, "nested", "b.txt"))
	if err != nil {
		t.Fatalf("reading nested/b.txt: %v", err)
	}
	if string(gotB) != "bbb" {
		t.Errorf("nested/b.txt = %q, want %q", gotB, "bbb")
	}
}

func TestCopyPath_NonexistentSrc(t *testing.T) {
	tmp := t.TempDir()
	err := CopyPath(filepath.Join(tmp, "nope"), filepath.Join(tmp, "dst"))
	if err == nil {
		t.Fatal("expected error for nonexistent src")
	}
}
