package commands

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/hay-kot/mmdot/internal/actions"
	"github.com/hay-kot/mmdot/internal/brew"
	"github.com/hay-kot/mmdot/internal/generator"
	"github.com/rs/zerolog/log"
)

type ConfigFile struct {
	Exec      actions.ExecConfig        `toml:"exec"`
	Bundles   map[string]actions.Bundle `toml:"bundles"`
	Actions   map[string]actions.Action `toml:"actions"`
	Brews     brew.ConfigMap            `toml:"brew"`
	Templates generator.Config          `toml:"templates"`
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
