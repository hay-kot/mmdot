package brew

import (
	"fmt"
	"strings"
)

type Config struct {
	Remove   bool     `toml:"remove"`
	Outfile  string   `toml:"outfile"`
	Includes []string `toml:"includes"`
	Brews    []string `toml:"brews"`
	Taps     []string `toml:"taps"`
	Casks    []string `toml:"casks"`
	MAS      []string `toml:"mas"`
}

// String returns a shell script to install the taps and brews
func (c *Config) String() string {
	var script strings.Builder

	script.WriteString(`#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e
# Exit if any command in a pipeline fails (not just the last one)
set -o pipefail
# Treat unset variables as an error when substituting
set -u
`)

	actionTap := "tap"
	actionInstall := "install"
	if c.Remove {
		actionTap = "untap"
		actionInstall = "uninstall"
	}

	// Add taps - all in one command if there are any
	if len(c.Taps) > 0 {
		script.WriteString("# Adding Homebrew Taps\n")
		for _, tap := range c.Taps {
			script.WriteString(fmt.Sprintf("brew %s %s\n", actionTap, tap))
		}
		script.WriteString("\n")
	}

	// Install brews - all in one command if there are any
	if len(c.Brews) > 0 {
		script.WriteString("# Installing Homebrew Packages\n")
		if len(c.Brews) == 1 {
			script.WriteString(fmt.Sprintf("brew %s %s\n", actionInstall, c.Brews[0]))
		} else {
			script.WriteString(fmt.Sprintf("brew %s \\\n", actionInstall))
			for i, brew := range c.Brews {
				if i == len(c.Brews)-1 {
					script.WriteString(fmt.Sprintf("  %s\n", brew))
				} else {
					script.WriteString(fmt.Sprintf("  %s \\\n", brew))
				}
			}
		}
		script.WriteString("\n")
	}

	// Install casks - all in one command if there are any
	if len(c.Casks) > 0 {
		script.WriteString("# Installing Homebrew Casks\n")
		if len(c.Casks) == 1 {
			script.WriteString(fmt.Sprintf("brew %s --cask %s\n", actionInstall, c.Casks[0]))
		} else {
			script.WriteString(fmt.Sprintf("brew %s --cask \\\n", actionInstall))
			for i, cask := range c.Casks {
				if i == len(c.Casks)-1 {
					script.WriteString(fmt.Sprintf("  %s\n", cask))
				} else {
					script.WriteString(fmt.Sprintf("  %s \\\n", cask))
				}
			}
		}
		script.WriteString("\n")
	}

	// Install Mac App Store apps - unfortunately mas doesn't support batch installs
	if len(c.MAS) > 0 {
		script.WriteString("# Installing Mac App Store Apps\n")
		for _, app := range c.MAS {
			script.WriteString(fmt.Sprintf("mas install %s\n", app))
		}
	}

	return script.String()
}

type ConfigMap = map[string]*Config

func Get(cm map[string]*Config, key string) *Config {
	// If the key doesn't exist, return nil
	if _, exists := cm[key]; !exists {
		return nil
	}

	// Start with the base configuration
	baseConfig := cm[key]

	// Create a set to track processed configs to prevent circular includes
	processedConfigs := make(map[string]bool)
	processedConfigs[key] = true

	// Merge included configurations
	mergedConfig := &Config{
		Remove:  baseConfig.Remove,
		Outfile: baseConfig.Outfile,
		Brews:   make([]string, 0),
		Taps:    make([]string, 0),
		Casks:   make([]string, 0),
		MAS:     make([]string, 0),
	}

	// Recursively merge includes
	for _, include := range baseConfig.Includes {
		includedConfig := mergeIncludes(cm, include, processedConfigs)
		if includedConfig != nil {
			mergedConfig.Brews = append(mergedConfig.Brews, includedConfig.Brews...)
			mergedConfig.Taps = append(mergedConfig.Taps, includedConfig.Taps...)
			mergedConfig.Casks = append(mergedConfig.Casks, includedConfig.Casks...)
			mergedConfig.MAS = append(mergedConfig.MAS, includedConfig.MAS...)
		}
	}

	// Add base config items
	mergedConfig.Brews = append(mergedConfig.Brews, baseConfig.Brews...)
	mergedConfig.Taps = append(mergedConfig.Taps, baseConfig.Taps...)
	mergedConfig.Casks = append(mergedConfig.Casks, baseConfig.Casks...)
	mergedConfig.MAS = append(mergedConfig.MAS, baseConfig.MAS...)

	return mergedConfig
}

// Helper method to recursively merge included configurations
func mergeIncludes(cm map[string]*Config, key string, processed map[string]bool) *Config {
	// Check for circular dependency
	if processed[key] {
		return nil
	}

	// Get the configuration for the key
	config, exists := cm[key]
	if !exists {
		return nil
	}

	// Mark as processed
	processed[key] = true

	// Create a merged configuration
	mergedConfig := &Config{
		Brews: make([]string, 0),
		Taps:  make([]string, 0),
		Casks: make([]string, 0),
		MAS:   make([]string, 0),
	}

	// Recursively process includes first
	for _, include := range config.Includes {
		includedConfig := mergeIncludes(cm, include, processed)
		if includedConfig != nil {
			mergedConfig.Brews = append(mergedConfig.Brews, includedConfig.Brews...)
			mergedConfig.Taps = append(mergedConfig.Taps, includedConfig.Taps...)
			mergedConfig.Casks = append(mergedConfig.Casks, includedConfig.Casks...)
			mergedConfig.MAS = append(mergedConfig.MAS, includedConfig.MAS...)
		}
	}

	// Add current config items
	mergedConfig.Brews = append(mergedConfig.Brews, config.Brews...)
	mergedConfig.Taps = append(mergedConfig.Taps, config.Taps...)
	mergedConfig.Casks = append(mergedConfig.Casks, config.Casks...)
	mergedConfig.MAS = append(mergedConfig.MAS, config.MAS...)

	return mergedConfig
}
