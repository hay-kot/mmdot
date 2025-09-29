package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/BurntSushi/toml"
	"github.com/hay-kot/mmdot/internal/actions"
	"github.com/hay-kot/mmdot/internal/brew"
	"github.com/hay-kot/mmdot/internal/generator"
	"github.com/hay-kot/mmdot/internal/ssh"
	"github.com/hay-kot/mmdot/pkgs/fcrypt"
	"github.com/rs/zerolog/log"
)

type ConfigFile struct {
	Exec      actions.ExecConfig        `toml:"exec"`
	Bundles   map[string]actions.Bundle `toml:"bundles"`
	Actions   map[string]actions.Action `toml:"actions"`
	Brews     brew.ConfigMap            `toml:"brew"`
	Templates generator.Config          `toml:"templates"`
	SSH       ssh.Config                `toml:"ssh"`
	Age       Age                       `toml:"age"`
}

// Returns a list of all files that should to be encrypted
func (c ConfigFile) EncryptedFiles() []string {
	files := []string{}

	// Collect encrypted SSH host files
	for _, host := range c.SSH.Hosts {
		if host.EncryptedFile != "" {
			files = append(files, host.EncryptedFile)
		}
	}

	// Collect encrypted template vars files
	for _, job := range c.Templates.Jobs {
		if job.VarsFile != "" {
			files = append(files, job.VarsFile)
		}
	}

	return files
}

func setupEnv(cfgpath string) (ConfigFile, error) {
	cfg := ConfigFile{
		Exec:    actions.ExecConfig{},
		Brews:   map[string]*brew.Config{},
		Bundles: map[string]actions.Bundle{},
		Actions: map[string]actions.Action{},
	}
	absolutePath, err := filepath.Abs(cfgpath)
	if err != nil {
		return cfg, err
	}

	configDir := filepath.Dir(absolutePath)
	err = os.Chdir(configDir)
	if err != nil {
		return cfg, err
	}

	log.Debug().Str("cwd", configDir).Msg("setting working directory to config dir")

	// Set default for strict mode before decoding
	cfg.Templates.StrictMode = true

	_, err = toml.DecodeFile(cfgpath, &cfg)
	if err != nil {
		return cfg, err
	}

	// Resolve template paths relative to config directory
	for i := range cfg.Templates.Jobs {
		job := &cfg.Templates.Jobs[i]

		// Convert relative paths to absolute based on config directory
		if !filepath.IsAbs(job.Template) {
			job.Template = filepath.Join(configDir, job.Template)
		}

		if !filepath.IsAbs(job.Output) {
			job.Output = filepath.Join(configDir, job.Output)
		}
	}

	return cfg, nil
}

type Age struct {
	Recipients   []string `toml:"recipients"`
	IdentityFile string   `toml:"identity_file"`
}

func (a Age) ReadIdentity() (age.Identity, error) {
	// Read the private key from the identity file
	identityData, err := os.ReadFile(a.IdentityFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file %s: %w", a.IdentityFile, err)
	}

	// Parse the identity file, skipping comments and empty lines
	var keyLine string
	for _, line := range strings.Split(string(identityData), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			keyLine = line
			break
		}
	}

	if keyLine == "" {
		return nil, fmt.Errorf("no valid key found in identity file %s", a.IdentityFile)
	}

	identity, err := fcrypt.LoadPrivateKey(keyLine)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	return identity, nil
}
