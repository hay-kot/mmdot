package actions

import (
	"fmt"
	"slices"
)

// Script represents a single executable script with associated tags
type Script struct {
	Path string   `yaml:"path"`
	Tags []string `yaml:"tags"`
}

// Bundle represents a collection of scripts
type Bundle struct {
	Scripts []Script `yaml:"scripts"`
}

// Action represents a collection of bundles to be executed together
type Action struct {
	Bundles []string `yaml:"bundles"`
}

// ExecConfig represents the shell execution configuration
type ExecConfig struct {
	Shell string `yaml:"shell"`
}

// Config represents the top-level configuration structure
type Config struct {
	Exec ExecConfig `yaml:"exec"`
}

// GetScriptsForBundle returns all scripts for a given bundle name
func GetScriptsForBundle(bundles map[string]Bundle, bundleName string) ([]Script, error) {
	bundle, exists := bundles[bundleName]
	if !exists {
		return nil, fmt.Errorf("bundle '%s' not found in configuration", bundleName)
	}

	return bundle.Scripts, nil
}

// GetScriptsForAction returns all scripts for a given action name
func GetScriptsForAction(actions map[string]Action, bundles map[string]Bundle, actionName string) ([]Script, error) {
	action, exists := actions[actionName]
	if !exists {
		return nil, fmt.Errorf("action '%s' not found in configuration", actionName)
	}

	var scripts []Script

	// Collect scripts from all bundles associated with this action
	for _, bundleName := range action.Bundles {
		bundleScripts, err := GetScriptsForBundle(bundles, bundleName)
		if err != nil {
			return nil, err
		}
		scripts = append(scripts, bundleScripts...)
	}

	return scripts, nil
}

// FilterScriptsByTags returns scripts that match all specified tags
func FilterScriptsByTags(scripts []Script, tags []string) []Script {
	if len(tags) == 0 {
		return scripts
	}

	var filtered []Script

	for _, script := range scripts {
		// Check if script has all the required tags
		hasAllTags := true
		for _, requiredTag := range tags {
			found := slices.Contains(script.Tags, requiredTag)
			if !found {
				hasAllTags = false
				break
			}
		}

		if hasAllTags {
			filtered = append(filtered, script)
		}
	}

	return filtered
}
