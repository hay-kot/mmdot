package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/BurntSushi/toml"
	"github.com/hay-kot/mmdot/pkgs/fcrypt"
	"github.com/rs/zerolog/log"
)

type ConfigFile struct {
	Age       Age              `toml:"age"`
	Variables Variables        `toml:"variables"`
	Actions   []Action         `toml:"actions"`
	metadata  toml.MetaData    // stored for action creation
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

	md, err := toml.DecodeFile(cfgpath, &cfg)
	if err != nil {
		return cfg, err
	}

	cfg.metadata = md

	return cfg, nil
}

// MetaData returns the TOML metadata for the config file
func (c ConfigFile) MetaData() toml.MetaData {
	return c.metadata
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

type Variables struct {
	VarFiles []VarFile      `toml:"var_files"`
	Vars     map[string]any `toml:"vars"`
}

type VarFile struct {
	Path    string
	IsVault bool
}

// UnmarshalText implements custom unmarshaling for VarFile
func (vf *VarFile) UnmarshalText(data []byte) error {
	s := string(data)

	// Check if the path contains vault parameter
	if idx := strings.Index(s, "?vault="); idx != -1 {
		vf.Path = s[:idx]
		vaultValue := s[idx+7:] // Skip "?vault="
		vf.IsVault = vaultValue == "true"
	} else {
		vf.Path = s
		vf.IsVault = false
	}

	return nil
}

// MarshalText implements custom marshaling for VarFile
func (vf VarFile) MarshalText() ([]byte, error) {
	s := vf.Path
	if vf.IsVault {
		s += "?vault=true"
	}
	return []byte(s), nil
}
