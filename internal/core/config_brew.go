package core

type Brews struct {
	Remove   bool     `yaml:"remove"`
	Includes []string `yaml:"includes"`
	Brews    []string `yaml:"brews"`
	Taps     []string `yaml:"taps"`
	Casks    []string `yaml:"casks"`
	MAS      []string `yaml:"mas"`
}

type ConfigMap map[string]*Brews

func (cm ConfigMap) Get(key string) *Brews {
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
	mergedConfig := &Brews{
		Remove: baseConfig.Remove,
		Brews:  make([]string, 0),
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
func mergeIncludes(cm map[string]*Brews, key string, processed map[string]bool) *Brews {
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
	mergedConfig := &Brews{
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
