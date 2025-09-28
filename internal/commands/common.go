package commands

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/hay-kot/mmdot/internal/actions"
	"github.com/hay-kot/mmdot/internal/brew"
	"github.com/rs/zerolog/log"
)

type ConfigFile struct {
	Exec    actions.ExecConfig        `toml:"exec"`
	Bundles map[string]actions.Bundle `toml:"bundles"`
	Actions map[string]actions.Action `toml:"actions"`
	Brews   brew.ConfigMap            `toml:"brew"`
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

	err = os.Chdir(filepath.Dir(absolutePath))
	if err != nil {
		return cfg, err
	}

	log.Debug().Str("cwd", filepath.Dir(absolutePath)).Msg("setting working directory to config dir")

	_, err = toml.DecodeFile(cfgpath, &cfg)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}
