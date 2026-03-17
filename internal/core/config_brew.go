package core

type Brews struct {
	Remove   bool     `yaml:"remove"`
	Includes []string `yaml:"includes"`
	Brews    []string `yaml:"brews"`
	Taps     []string `yaml:"taps"`
	Casks    []string `yaml:"casks"`
	MAS      []string `yaml:"mas"`
}

func (b *Brews) merge(other *Brews) {
	b.Brews = append(b.Brews, other.Brews...)
	b.Taps = append(b.Taps, other.Taps...)
	b.Casks = append(b.Casks, other.Casks...)
	b.MAS = append(b.MAS, other.MAS...)
}

type ConfigMap map[string]*Brews

func (cm ConfigMap) Get(key string) *Brews {
	if _, exists := cm[key]; !exists {
		return nil
	}

	baseConfig := cm[key]

	// Track processed configs to prevent circular includes
	processedConfigs := make(map[string]bool)
	processedConfigs[key] = true

	mergedConfig := &Brews{
		Remove: baseConfig.Remove,
		Brews:  make([]string, 0),
		Taps:   make([]string, 0),
		Casks:  make([]string, 0),
		MAS:    make([]string, 0),
	}

	for _, include := range baseConfig.Includes {
		if included := mergeIncludes(cm, include, processedConfigs); included != nil {
			mergedConfig.merge(included)
		}
	}

	mergedConfig.merge(baseConfig)

	return mergedConfig
}

func mergeIncludes(cm map[string]*Brews, key string, processed map[string]bool) *Brews {
	if processed[key] {
		return nil
	}

	config, exists := cm[key]
	if !exists {
		return nil
	}

	processed[key] = true

	mergedConfig := &Brews{
		Brews: make([]string, 0),
		Taps:  make([]string, 0),
		Casks: make([]string, 0),
		MAS:   make([]string, 0),
	}

	for _, include := range config.Includes {
		if included := mergeIncludes(cm, include, processed); included != nil {
			mergedConfig.merge(included)
		}
	}

	mergedConfig.merge(config)

	return mergedConfig
}
