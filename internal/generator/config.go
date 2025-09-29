package generator

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
