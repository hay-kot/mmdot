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
	Groups    Groups     `yaml:"groups"`
	Exec      Exec       `yaml:"exec"`
	Age       Age        `yaml:"age"`
	Brews     ConfigMap  `yaml:"brews"`
	Variables Variables  `yaml:"variables"`
	Templates []Template `yaml:"templates"`
	ConfigDir string     `yaml:"-"` // Directory containing the config file (not serialized)
}

// Groups maps group names to their configuration
type Groups map[string]Group

// Group represents a named group with associated tags
type Group struct {
	Tags []string `yaml:"tags"`
}

// ExecConfig represents the shell execution configuration
type Exec struct {
	Shell   string   `yaml:"shell"`
	Scripts []Script `yaml:"scripts"`
}

// Script represents a single executable script with associated tags
type Script struct {
	Path string   `yaml:"path"`
	Tags []string `yaml:"tags"`
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
	cfg.ConfigDir = configDir

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

	// Create path resolver and resolve all paths in config
	pr := PathResolver{configDir: configDir}
	err = cfg.resolvePaths(pr)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}

// resolvePaths resolves all path properties in the config using the PathResolver
func (c *ConfigFile) resolvePaths(pr PathResolver) error {
	// Resolve Age identity file path
	if c.Age.IdentityFile != "" {
		resolved, err := pr.Resolve(c.Age.IdentityFile)
		if err != nil {
			return fmt.Errorf("failed to resolve age identity file path: %w", err)
		}
		c.Age.IdentityFile = resolved
	}

	// Resolve variable file paths
	for i := range c.Variables.VarFiles {
		resolved, err := pr.Resolve(c.Variables.VarFiles[i].Path)
		if err != nil {
			return fmt.Errorf("failed to resolve var file path: %w", err)
		}
		c.Variables.VarFiles[i].Path = resolved
	}

	// Resolve template paths (template input and output)
	for i := range c.Templates {

		if c.Templates[i].Template != "" && !strings.Contains(c.Templates[i].Template, "{{") {
			resolved, err := pr.Resolve(c.Templates[i].Template)
			if err != nil {
				return fmt.Errorf("failed to resolve template path: %w", err)
			}
			c.Templates[i].Template = resolved
		}
		if c.Templates[i].Output != "" {
			resolved, err := pr.Resolve(c.Templates[i].Output)
			if err != nil {
				return fmt.Errorf("failed to resolve template output path: %w", err)
			}
			c.Templates[i].Output = resolved
		}
	}

	// Resolve exec script paths
	for i := range c.Exec.Scripts {
		resolved, err := pr.Resolve(c.Exec.Scripts[i].Path)
		if err != nil {
			return fmt.Errorf("failed to resolve exec script path: %w", err)
		}
		c.Exec.Scripts[i].Path = resolved
	}

	return nil
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

func (vf *VarFile) UnmarshalYAML(unmarshal func(any) error) error {
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
	Trim        *bool          `yaml:"trim"` // Trim leading/trailing whitespace from output (default: true)
}

func (t Template) ShouldTrim() bool {
	if t.Trim == nil {
		return true // Default to true
	}
	return *t.Trim
}
