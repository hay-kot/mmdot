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
	Exec      Exec       `yaml:"exec"`
	Age       Age        `yaml:"age"`
	Brews     ConfigMap  `yaml:"brews"`
	Variables Variables  `yaml:"variables"`
	Templates []Template `yaml:"templates"`
}

// ExecConfig represents the shell execution configuration
type Exec struct {
	Shell   string   `toml:"shell"`
	Scripts []Script `toml:"scripts"`
}

// Script represents a single executable script with associated tags
type Script struct {
	Path string   `toml:"path"`
	Tags []string `toml:"tags"`
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
	Path    string
	IsVault bool
}

func (vf *VarFile) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try unmarshaling as a string first
	var path string
	if err := unmarshal(&path); err == nil {
		// Parse query parameters if present
		if idx := strings.Index(path, "?"); idx != -1 {
			vf.Path = path[:idx]
			query := path[idx+1:]
			// Check for vault=true
			vf.IsVault = strings.Contains(query, "vault=true")
		} else {
			vf.Path = path
			vf.IsVault = false
		}
		return nil
	}

	// Fall back to struct format
	var v struct {
		Path    string `yaml:"path"`
		IsVault bool   `yaml:"vault"`
	}
	if err := unmarshal(&v); err != nil {
		return err
	}
	vf.Path = v.Path
	vf.IsVault = v.IsVault
	return nil
}

type Template struct {
	Name        string         `yaml:"name"`
	Tags        []string       `yaml:"tags"`
	Template    string         `yaml:"template"` // File or Template
	Output      string         `yaml:"output"`
	Permissions string         `yaml:"perm"` // Must be valid permissions
	Vars        map[string]any `yaml:"vars"`
}
