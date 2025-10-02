package generator

type Config struct {
	Jobs       []Job          `yaml:"jobs"`
	Vars       map[string]any `yaml:"vars,omitempty"`
	StrictMode bool           `yaml:"strict_mode"`
}

type Job struct {
	Template string         `yaml:"template"`
	Output   string         `yaml:"output"`
	Vars     map[string]any `yaml:"vars,omitempty"`
	VarsFile string         `yaml:"vars_file"` // VarsFile assumed to be an encrypted file
}
