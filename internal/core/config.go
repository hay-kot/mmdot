package core

type Exec struct {
	Shell   string   `toml:"shell"`
	Scripts []Script `toml:"scripts"`
}

type Script struct {
	Path string   `toml:"path"`
	Tags []string `toml:"tags"`
}
