package main

import (
	"github.com/hay-kot/mmdot/internal/actions"
	"github.com/hay-kot/mmdot/internal/brew"
)

type ConfigFile struct {
	Exec    actions.ExecConfig        `toml:"exec"`
	Bundles map[string]actions.Bundle `toml:"bundles"`
	Actions map[string]actions.Action `toml:"actions"`
	Brews   brew.ConfigMap            `toml:"brew"`
}
