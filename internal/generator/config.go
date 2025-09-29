package generator

import (
	"maps"
)

type Config struct {
	Jobs       []Job          `toml:"jobs"`
	Vars       map[string]any `toml:"vars,omitempty"`
	StrictMode bool           `toml:"strict_mode"`
}

type Job struct {
	Template string         `toml:"template"`
	Output   string         `toml:"output"`
	Vars     map[string]any `toml:"vars,omitempty"`
	VarsFile string         `toml:"vars_file"` // VarsFile assumed to be an encrypted file
}

func (j *Job) MergeVars(globalVars map[string]any) map[string]any {
	merged := make(map[string]any)
	maps.Copy(merged, globalVars)
	maps.Copy(merged, j.Vars)
	return merged
}

