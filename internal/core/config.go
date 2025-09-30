package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/goccy/go-yaml"
	"github.com/hay-kot/mmdot/pkgs/fcrypt"
	"github.com/rs/zerolog/log"
)

type ConfigFile struct {
	Age       Age       `yaml:"age"`
	Variables Variables `yaml:"variables"`
	Actions   []Action  `yaml:"actions"`
}

func SetupEnv(cfgpath string) (ConfigFile, error) {
	cfg := ConfigFile{
		Age:       Age{},
		Variables: Variables{},
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

	data, err := os.ReadFile(cfgpath)
	if err != nil {
		return cfg, err
	}

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}

// Returns a list of all files that should to be encrypted
func (c ConfigFile) EncryptedFiles() []string {
	files := []string{}

	for _, vf := range c.Variables.VarFiles {
		if vf.IsVault {
			files = append(files, vf.Path)
		}
	}

	return files
}

type Age struct {
	Recipients   []string `yaml:"recipients"`
	IdentityFile string   `yaml:"identity_file"`
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

type Variables struct {
	VarFiles []VarFile      `yaml:"var_files"`
	Vars     map[string]any `yaml:"vars"`
}

type VarFile struct {
	Path    string `yaml:"path"`
	IsVault bool   `yaml:"vault"`
}
