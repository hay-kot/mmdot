package generator

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/pkgs/fcrypt"
)

func TestBrewfilePartial(t *testing.T) {
	dir := t.TempDir()
	outfile := filepath.Join(dir, "brew.sh")

	cfg := &core.ConfigFile{
		Brews: core.ConfigMap{
			"base": &core.Brews{
				Brews: []string{"curl", "wget"},
			},
			"personal": &core.Brews{
				Includes: []string{"base"},
				Taps:     []string{"homebrew/cask"},
				Brews:    []string{"git", "vim"},
				Casks:    []string{"firefox"},
				MAS:      []string{"497799835"},
			},
		},
		Variables: core.Variables{},
	}

	engine := NewEngine(cfg)

	tmpl := core.Template{
		Name:   "test-brewfile",
		Output: outfile,
		Template: `#!/bin/bash
set -euo pipefail
{{template "brewfile" "personal"}}`,
	}

	err := engine.RenderTemplate(context.Background(), tmpl)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	got, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	output := string(got)

	// Verify includes are resolved (base brews merged in)
	for _, want := range []string{
		"brew tap homebrew/cask",
		"brew install \\\n  curl",
		"  curl \\\n  wget",
		"  vim\n",
		"brew install --cask \\\n  firefox",
		"mas install 497799835",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("output missing %q\n\ngot:\n%s", want, output)
		}
	}
}

func TestBrewfilePartialRemove(t *testing.T) {
	dir := t.TempDir()
	outfile := filepath.Join(dir, "brew-remove.sh")

	cfg := &core.ConfigFile{
		Brews: core.ConfigMap{
			"cleanup": &core.Brews{
				Remove: true,
				Taps:   []string{"old/tap"},
				Brews:  []string{"oldpkg"},
				Casks:  []string{"oldcask"},
				MAS:    []string{"123456"},
			},
		},
		Variables: core.Variables{},
	}

	engine := NewEngine(cfg)

	tmpl := core.Template{
		Name:     "test-remove",
		Output:   outfile,
		Template: `{{template "brewfile" "cleanup"}}`,
	}

	err := engine.RenderTemplate(context.Background(), tmpl)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	got, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	output := string(got)

	for _, want := range []string{
		"brew untap old/tap",
		"brew uninstall oldpkg",
		"brew uninstall --cask oldcask",
		"mas uninstall 123456",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("output missing %q\n\ngot:\n%s", want, output)
		}
	}
}

func TestBrewfilePartialUnknownConfig(t *testing.T) {
	dir := t.TempDir()
	outfile := filepath.Join(dir, "out.sh")

	cfg := &core.ConfigFile{
		Brews:     core.ConfigMap{},
		Variables: core.Variables{},
	}

	engine := NewEngine(cfg)

	tmpl := core.Template{
		Name:     "test-unknown",
		Output:   outfile,
		Template: `{{template "brewfile" "nonexistent"}}`,
	}

	err := engine.RenderTemplate(context.Background(), tmpl)
	if err == nil {
		t.Fatal("expected error for unknown brew config, got nil")
	}
}

// generateTestIdentity creates a fresh age X25519 identity for use in tests.
func generateTestIdentity(t *testing.T) *age.X25519Identity {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate test identity: %v", err)
	}
	return id
}

// writeEncryptedVault writes content to path.age encrypted to recipients.
func writeEncryptedVault(t *testing.T, plainPath string, content []byte, recipients []age.Recipient) {
	t.Helper()
	encPath := plainPath + ".age"
	if err := os.WriteFile(plainPath, content, 0o644); err != nil {
		t.Fatalf("write plaintext for encryption: %v", err)
	}
	if err := fcrypt.EncryptFileKeepSource(plainPath, encPath, recipients); err != nil {
		t.Fatalf("encrypt vault: %v", err)
	}
}

// TestLoadVarsFile_VaultFallback_DecryptionFails_PlaintextPresent verifies that
// when the .age file cannot be decrypted (wrong recipient) AND the plaintext
// file exists, preloadVars succeeds and returns the plaintext variables.
func TestLoadVarsFile_VaultFallback_DecryptionFails_PlaintextPresent(t *testing.T) {
	dir := t.TempDir()
	vaultBase := filepath.Join(dir, "vault.yml")

	// Encrypt with a key the engine will NOT have.
	otherID := generateTestIdentity(t)
	plain := []byte("secret_key: secret_value\n")
	writeEncryptedVault(t, vaultBase, plain, []age.Recipient{otherID.Recipient()})

	// Engine identity is a fresh key — it does NOT match the recipient above.
	engineID := generateTestIdentity(t)
	identityFile := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(identityFile, []byte(engineID.String()+"\n"), 0o600); err != nil {
		t.Fatalf("write identity file: %v", err)
	}

	cfg := &core.ConfigFile{
		Age: core.Age{IdentityFile: identityFile},
		Variables: core.Variables{
			VarFiles: []core.VarFile{
				{Path: vaultBase, IsVault: true, Strict: false},
			},
		},
	}

	engine := NewEngine(cfg)
	if err := engine.preloadVars(); err != nil {
		t.Fatalf("preloadVars should succeed via plaintext fallback, got: %v", err)
	}

	if got := engine.fileVars["secret_key"]; got != "secret_value" {
		t.Errorf("fileVars[secret_key] = %v, want secret_value", got)
	}
}

// TestLoadVarsFile_VaultFallback_DecryptionFails_NoPlaintext verifies that
// when the .age file cannot be decrypted AND there is no plaintext sibling,
// preloadVars returns an error that names the encrypted file, the identity
// file, and includes the mmdot encrypt hint.
func TestLoadVarsFile_VaultFallback_DecryptionFails_NoPlaintext(t *testing.T) {
	dir := t.TempDir()
	vaultBase := filepath.Join(dir, "vault.yml")

	// Create only the .age file, encrypted to a key we don't have.
	otherID := generateTestIdentity(t)
	encPath := vaultBase + ".age"
	if err := os.WriteFile(encPath, nil, 0o644); err != nil {
		t.Fatalf("create placeholder age file: %v", err)
	}
	// Overwrite with a proper encrypted payload.
	plain := []byte("key: value\n")
	if err := os.WriteFile(vaultBase, plain, 0o644); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}
	if err := fcrypt.EncryptFileKeepSource(vaultBase, encPath, []age.Recipient{otherID.Recipient()}); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// Remove plaintext so only .age exists.
	if err := os.Remove(vaultBase); err != nil {
		t.Fatalf("remove plaintext: %v", err)
	}

	engineID := generateTestIdentity(t)
	identityFile := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(identityFile, []byte(engineID.String()+"\n"), 0o600); err != nil {
		t.Fatalf("write identity file: %v", err)
	}

	cfg := &core.ConfigFile{
		Age: core.Age{IdentityFile: identityFile},
		Variables: core.Variables{
			VarFiles: []core.VarFile{
				{Path: vaultBase, IsVault: true, Strict: false},
			},
		},
	}

	engine := NewEngine(cfg)
	err := engine.preloadVars()
	if err == nil {
		t.Fatal("expected error when .age undecryptable and no plaintext, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, encPath) {
		t.Errorf("error should mention encrypted path %q, got: %s", encPath, errStr)
	}
	if !strings.Contains(errStr, identityFile) {
		t.Errorf("error should mention identity file %q, got: %s", identityFile, errStr)
	}
	if !strings.Contains(errStr, "mmdot encrypt --force") {
		t.Errorf("error should include 'mmdot encrypt --force' hint, got: %s", errStr)
	}
}

// TestLoadVarsFile_VaultStrict_DecryptionFails verifies that when strict mode
// is enabled, a decryption failure is always fatal even if the plaintext
// sibling exists.
func TestLoadVarsFile_VaultStrict_DecryptionFails(t *testing.T) {
	dir := t.TempDir()
	vaultBase := filepath.Join(dir, "vault.yml")

	otherID := generateTestIdentity(t)
	plain := []byte("secret_key: must_not_leak\n")
	writeEncryptedVault(t, vaultBase, plain, []age.Recipient{otherID.Recipient()})

	engineID := generateTestIdentity(t)
	identityFile := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(identityFile, []byte(engineID.String()+"\n"), 0o600); err != nil {
		t.Fatalf("write identity file: %v", err)
	}

	cfg := &core.ConfigFile{
		Age: core.Age{IdentityFile: identityFile},
		Variables: core.Variables{
			VarFiles: []core.VarFile{
				{Path: vaultBase, IsVault: true, Strict: true},
			},
		},
	}

	engine := NewEngine(cfg)
	err := engine.preloadVars()
	if err == nil {
		t.Fatal("expected error in strict mode, got nil")
	}
	if !strings.Contains(err.Error(), "strict mode") {
		t.Errorf("error should mention 'strict mode', got: %s", err)
	}
}

// TestLoadVarsFile_VaultSuccess verifies that a correctly-encrypted vault
// is still decrypted successfully (the happy path is not broken).
func TestLoadVarsFile_VaultSuccess(t *testing.T) {
	dir := t.TempDir()
	vaultBase := filepath.Join(dir, "vault.yml")

	id := generateTestIdentity(t)
	plain := []byte("db_password: hunter2\n")
	writeEncryptedVault(t, vaultBase, plain, []age.Recipient{id.Recipient()})

	identityFile := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(identityFile, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatalf("write identity file: %v", err)
	}

	cfg := &core.ConfigFile{
		Age: core.Age{IdentityFile: identityFile},
		Variables: core.Variables{
			VarFiles: []core.VarFile{
				{Path: vaultBase, IsVault: true},
			},
		},
	}

	engine := NewEngine(cfg)
	if err := engine.preloadVars(); err != nil {
		t.Fatalf("preloadVars failed: %v", err)
	}
	if got := engine.fileVars["db_password"]; got != "hunter2" {
		t.Errorf("fileVars[db_password] = %v, want hunter2", got)
	}
}
