package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/pkgs/fcrypt"
	"github.com/urfave/cli/v3"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("warning: failed to restore working directory: %v", err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir to %s: %v", dir, err)
	}
}

func Test_ensureGitignored(t *testing.T) {
	tmpDir := t.TempDir()
	chdir(t, tmpDir)

	path := "output/secret.md"

	// First call should create .gitignore and add the path
	if err := ensureGitignored(path); err != nil {
		t.Fatalf("first ensureGitignored() error: %v", err)
	}

	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if string(data) != path+"\n" {
		t.Errorf(".gitignore content = %q, want %q", string(data), path+"\n")
	}

	// Second call should be a no-op (path already present)
	if err := ensureGitignored(path); err != nil {
		t.Fatalf("second ensureGitignored() error: %v", err)
	}

	data, _ = os.ReadFile(".gitignore")
	if string(data) != path+"\n" {
		t.Errorf("after second call, .gitignore content = %q, want %q", string(data), path+"\n")
	}
}

func Test_ensureGitignored_existingContentNoTrailingNewline(t *testing.T) {
	tmpDir := t.TempDir()
	chdir(t, tmpDir)

	// Create existing .gitignore without trailing newline
	if err := os.WriteFile(".gitignore", []byte("*.log"), 0o644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	path := "output/secret.md"
	if err := ensureGitignored(path); err != nil {
		t.Fatalf("ensureGitignored() error: %v", err)
	}

	data, _ := os.ReadFile(".gitignore")
	want := "*.log\n" + path + "\n"
	if string(data) != want {
		t.Errorf(".gitignore content = %q, want %q", string(data), want)
	}
}

func Test_ensureGitignored_existingContentWithTrailingNewline(t *testing.T) {
	tmpDir := t.TempDir()
	chdir(t, tmpDir)

	// Create existing .gitignore with trailing newline
	if err := os.WriteFile(".gitignore", []byte("*.log\n"), 0o644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	path := "output/secret.md"
	if err := ensureGitignored(path); err != nil {
		t.Fatalf("ensureGitignored() error: %v", err)
	}

	data, _ := os.ReadFile(".gitignore")
	want := "*.log\n" + path + "\n"
	if string(data) != want {
		t.Errorf(".gitignore content = %q, want %q", string(data), want)
	}
}

func testRecipients(t *testing.T) (*age.X25519Identity, []age.Recipient) {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	return id, []age.Recipient{id.Recipient()}
}

// secretFixture is a plaintext file plus its .age counterpart, wired into a
// config as a single age.files entry.
type secretFixture struct {
	plaintext string
	encrypted string
	identity  *age.X25519Identity
	cfg       core.ConfigFile
}

func newSecretFixture(t *testing.T, content string) secretFixture {
	t.Helper()

	id, recipients := testRecipients(t)
	dir := t.TempDir()

	f := secretFixture{
		plaintext: filepath.Join(dir, "secret.yml"),
		encrypted: filepath.Join(dir, "secret.yml.age"),
		identity:  id,
	}
	f.cfg = core.ConfigFile{
		Age: core.Age{
			Recipients: []string{"recipient-1"},
			Files:      []core.AgeFile{{Src: f.encrypted, Dest: f.plaintext}},
		},
	}

	f.writePlaintext(t, content)
	if err := fcrypt.EncryptFileKeepSource(f.plaintext, f.encrypted, recipients); err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}

	return f
}

func (f secretFixture) writePlaintext(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(f.plaintext, []byte(content), 0o600); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}
}

func (f secretFixture) loader() identityLoader {
	return func() (age.Identity, error) { return f.identity, nil }
}

// failingLoader stands in for a machine without a readable identity file.
var failingLoader identityLoader = func() (age.Identity, error) {
	return nil, fmt.Errorf("identity file not readable")
}

// touchFuture backdates every other file by pushing this one an hour ahead, so
// a plan that still consults mtimes would reach the wrong conclusion.
func touchFuture(t *testing.T, path string) {
	t.Helper()
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func TestPlanEncryption_EditedPlaintextNeedsReencryption(t *testing.T) {
	f := newSecretFixture(t, "hostname: old\n")
	f.writePlaintext(t, "hostname: new\n")
	touchFuture(t, f.encrypted)

	actions, err := (&EncryptCmd{}).planEncryption(f.cfg, f.loader())
	if err != nil {
		t.Fatalf("planEncryption: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %v, want one", actions)
	}
	if actions[0].reason != reasonPlaintextEdit {
		t.Errorf("reason = %q, want %q", actions[0].reason, reasonPlaintextEdit)
	}
	if !actions[0].fromPlaintext {
		t.Error("edited file must be re-encrypted from its plaintext source")
	}
}

func TestPlanEncryption_UnchangedPlaintextIsNoop(t *testing.T) {
	f := newSecretFixture(t, "hostname: old\n")
	touchFuture(t, f.plaintext)

	actions, err := (&EncryptCmd{}).planEncryption(f.cfg, f.loader())
	if err != nil {
		t.Fatalf("planEncryption: %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("actions = %v, want none", actions)
	}
}

func TestPlanEncryption_MissingEncryptedFile(t *testing.T) {
	f := newSecretFixture(t, "hostname: old\n")
	if err := os.Remove(f.encrypted); err != nil {
		t.Fatal(err)
	}

	// No ciphertext to compare against, so no identity should be needed.
	actions, err := (&EncryptCmd{}).planEncryption(f.cfg, failingLoader)
	if err != nil {
		t.Fatalf("planEncryption: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %v, want one", actions)
	}
	if actions[0].reason != reasonNotEncrypted || !actions[0].fromPlaintext {
		t.Errorf("action = %+v, want %q from plaintext", actions[0], reasonNotEncrypted)
	}
}

func TestPlanEncryption_UnavailableIdentityFailsLoudly(t *testing.T) {
	f := newSecretFixture(t, "hostname: old\n")
	f.writePlaintext(t, "hostname: new\n")

	actions, err := (&EncryptCmd{}).planEncryption(f.cfg, failingLoader)
	if err == nil {
		t.Fatalf("planEncryption = %v, want an error when the identity is unavailable", actions)
	}
}

func TestPlanEncryption_UndecryptableCiphertextFailsLoudly(t *testing.T) {
	f := newSecretFixture(t, "hostname: old\n")

	// A .age file encrypted to somebody else's key: the change check can't run,
	// and skipping it silently is exactly the failure mode being fixed.
	_, otherRecipients := testRecipients(t)
	if err := fcrypt.EncryptFileKeepSource(f.plaintext, f.encrypted, otherRecipients); err != nil {
		t.Fatal(err)
	}

	if _, err := (&EncryptCmd{}).planEncryption(f.cfg, f.loader()); err == nil {
		t.Fatal("planEncryption succeeded, want an error when the .age file can't be decrypted")
	}
}

func TestPlanEncryption_ForceReencryptsFromPlaintext(t *testing.T) {
	f := newSecretFixture(t, "hostname: old\n")

	// force skips the content comparison, so it must not need the identity to
	// plan work for a file that has a plaintext source.
	actions, err := (&EncryptCmd{force: true}).planEncryption(f.cfg, failingLoader)
	if err != nil {
		t.Fatalf("planEncryption: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %v, want one", actions)
	}
	if actions[0].reason != reasonForced {
		t.Errorf("reason = %q, want %q", actions[0].reason, reasonForced)
	}
	if !actions[0].fromPlaintext {
		t.Error("force must re-encrypt from plaintext, not round-trip the old ciphertext")
	}
}

func TestPlanEncryption_RecipientCountChange(t *testing.T) {
	f := newSecretFixture(t, "hostname: old\n")
	f.cfg.Age.Recipients = append(f.cfg.Age.Recipients, "recipient-2")

	actions, err := (&EncryptCmd{}).planEncryption(f.cfg, f.loader())
	if err != nil {
		t.Fatalf("planEncryption: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %v, want one", actions)
	}
	if actions[0].reason != reasonRecipients {
		t.Errorf("reason = %q, want %q", actions[0].reason, reasonRecipients)
	}
}

func TestPlanEncryption_RotatesCiphertextWithoutPlaintext(t *testing.T) {
	f := newSecretFixture(t, "hostname: old\n")
	if err := os.Remove(f.plaintext); err != nil {
		t.Fatal(err)
	}
	f.cfg.Age.Recipients = append(f.cfg.Age.Recipients, "recipient-2")

	actions, err := (&EncryptCmd{}).planEncryption(f.cfg, f.loader())
	if err != nil {
		t.Fatalf("planEncryption: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %v, want one", actions)
	}
	if actions[0].reason != reasonRecipients {
		t.Errorf("reason = %q, want %q", actions[0].reason, reasonRecipients)
	}
	if actions[0].fromPlaintext {
		t.Error("without a plaintext copy the .age file must be rotated in place")
	}
}

// encryptEnv is a temp directory holding an mmdot config, an age identity and
// the commands wired to them.
type encryptEnv struct {
	dir      string
	identity *age.X25519Identity
	cmd      *EncryptCmd
	dryRun   *EncryptCmd
}

// newEncryptEnv writes an mmdot config and identity file into a temp directory.
// configBody is appended below the generated age block, so it can either
// continue it (a "  files:" list) or open a new top-level section.
func newEncryptEnv(t *testing.T, configBody string) encryptEnv {
	t.Helper()

	id, _ := testRecipients(t)
	dir := t.TempDir()
	chdir(t, dir)

	keyPath := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(keyPath, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	config := fmt.Sprintf(`version: 2
age:
  identity_file: key.txt
  recipients:
    - %s
%s`, id.Recipient(), configBody)

	configPath := filepath.Join(dir, "mmdot.yml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	flags := &core.Flags{ConfigFilePath: configPath}
	return encryptEnv{
		dir:      dir,
		identity: id,
		cmd:      NewEncryptCmd(flags),
		dryRun:   &EncryptCmd{coreFlags: flags, dryRun: true},
	}
}

func (e encryptEnv) path(name string) string {
	return filepath.Join(e.dir, name)
}

func (e encryptEnv) readEncrypted(t *testing.T, name string) string {
	t.Helper()

	f, err := os.Open(e.path(name))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer func() { _ = f.Close() }()

	var out bytes.Buffer
	if err := fcrypt.DecryptReader(f, &out, e.identity); err != nil {
		t.Fatalf("decrypt %s: %v", name, err)
	}
	return out.String()
}

func mustRun(t *testing.T, step string, fn func(context.Context, *cli.Command) error) {
	t.Helper()
	if err := fn(context.Background(), &cli.Command{}); err != nil {
		t.Fatalf("%s: %v", step, err)
	}
}

// The documented workflow — decrypt, edit the plaintext, encrypt — used to
// discard the edit because encrypt skipped any secret whose .age file existed.
func TestEncrypt_VaultWorkflowPreservesEdits(t *testing.T) {
	env := newEncryptEnv(t, `variables:
  var_files:
    - vault.yml?vault=true
`)

	if err := os.WriteFile(env.path("vault.yml"), []byte("hostname: old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	mustRun(t, "initial encrypt", env.cmd.encrypt)
	if _, err := os.Stat(env.path("vault.yml")); !os.IsNotExist(err) {
		t.Fatalf("plaintext vault file should be removed after encrypt, stat err = %v", err)
	}

	mustRun(t, "decrypt", env.cmd.decrypt)
	if err := os.WriteFile(env.path("vault.yml"), []byte("hostname: new\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := env.dryRun.encrypt(context.Background(), &cli.Command{}); err == nil {
		t.Error("dry-run reported nothing to do after the plaintext was edited")
	}

	mustRun(t, "encrypt after edit", env.cmd.encrypt)

	if got := env.readEncrypted(t, "vault.yml.age"); got != "hostname: new\n" {
		t.Errorf("encrypted vault content = %q, want the edited value", got)
	}
	if _, err := os.Stat(env.path("vault.yml")); !os.IsNotExist(err) {
		t.Errorf("plaintext vault file should be removed after re-encrypt, stat err = %v", err)
	}

	// The edited value must survive the full round trip.
	mustRun(t, "decrypt after edit", env.cmd.decrypt)
	restored, err := os.ReadFile(env.path("vault.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != "hostname: new\n" {
		t.Errorf("round-tripped vault content = %q, want the edited value", restored)
	}
}

func TestEncrypt_AgeFileWorkflowPreservesEdits(t *testing.T) {
	env := newEncryptEnv(t, `  files:
    - src: secrets/example.age
      dest: example.txt
`)

	if err := os.WriteFile(env.path("example.txt"), []byte("token: old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	mustRun(t, "initial encrypt", env.cmd.encrypt)

	// Encrypting again with an untouched plaintext is a no-op, so the file on
	// disk must not change even though age would produce fresh ciphertext.
	before, err := os.ReadFile(env.path("secrets/example.age"))
	if err != nil {
		t.Fatal(err)
	}
	mustRun(t, "no-op encrypt", env.cmd.encrypt)
	after, err := os.ReadFile(env.path("secrets/example.age"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("unchanged plaintext should not rewrite the .age file")
	}

	if err := os.WriteFile(env.path("example.txt"), []byte("token: new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "encrypt after edit", env.cmd.encrypt)

	if got := env.readEncrypted(t, "secrets/example.age"); got != "token: new\n" {
		t.Errorf("encrypted content = %q, want the edited value", got)
	}
	plaintext, err := os.ReadFile(env.path("example.txt"))
	if err != nil {
		t.Fatalf("age.files plaintext dest was removed: %v", err)
	}
	if string(plaintext) != "token: new\n" {
		t.Errorf("plaintext dest = %q, want it left untouched", plaintext)
	}
}

func TestEncrypt_ForceReencryptsCurrentPlaintext(t *testing.T) {
	env := newEncryptEnv(t, `  files:
    - src: secrets/example.age
      dest: example.txt
`)

	if err := os.WriteFile(env.path("example.txt"), []byte("token: old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "initial encrypt", env.cmd.encrypt)

	if err := os.WriteFile(env.path("example.txt"), []byte("token: new\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	force := &EncryptCmd{coreFlags: env.cmd.coreFlags, force: true}
	mustRun(t, "force encrypt", force.encrypt)

	if got := env.readEncrypted(t, "secrets/example.age"); got != "token: new\n" {
		t.Errorf("force encrypted content = %q, want the current plaintext", got)
	}
}

func TestEncryptFileKeepSource(t *testing.T) {
	_, recipients := testRecipients(t)

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "secret.json")
	outputPath := filepath.Join(tmpDir, "secret.age")

	const plaintext = "sensitive data"
	if err := os.WriteFile(inputPath, []byte(plaintext), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := fcrypt.EncryptFileKeepSource(inputPath, outputPath, recipients); err != nil {
		t.Fatalf("EncryptFileKeepSource: %v", err)
	}

	// Plaintext source must still exist
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("plaintext source was deleted: %v", err)
	}
	if string(data) != plaintext {
		t.Errorf("plaintext content changed: got %q, want %q", data, plaintext)
	}

	// Encrypted output must exist
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("encrypted output missing: %v", err)
	}
}

func TestEncryptFileKeepSource_RoundTrip(t *testing.T) {
	id, recipients := testRecipients(t)

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "config.json")
	encPath := filepath.Join(tmpDir, "config.age")
	decPath := filepath.Join(tmpDir, "config-restored.json")

	const plaintext = `{"key": "value"}`
	if err := os.WriteFile(inputPath, []byte(plaintext), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := fcrypt.EncryptFileKeepSource(inputPath, encPath, recipients); err != nil {
		t.Fatalf("EncryptFileKeepSource: %v", err)
	}

	if err := fcrypt.DecryptFile(encPath, decPath, id); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}

	got, _ := os.ReadFile(decPath)
	if string(got) != plaintext {
		t.Errorf("round-trip failed: got %q, want %q", got, plaintext)
	}
}
