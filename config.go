package main

import (
	"github.com/hay-kot/mmdot/internal/brew"
	"github.com/hay-kot/mmdot/internal/core"
)

type ConfigFile struct {
	Exec  core.Exec      `toml:"exec"`
	Brews brew.ConfigMap `toml:"brew"`
}
