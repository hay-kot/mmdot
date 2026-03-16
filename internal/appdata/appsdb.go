package appdata

import (
	_ "embed"

	"github.com/goccy/go-yaml"
)

//go:embed apps.yml
var appsYAML []byte

// AppDef defines an application's configuration files.
type AppDef struct {
	Name     string   `yaml:"name"`
	Files    []string `yaml:"files,omitempty"`
	XDGFiles []string `yaml:"xdg_files,omitempty"`
}

// LoadAppDB unmarshals the embedded apps.yml into a map keyed by application ID.
func LoadAppDB() (map[string]AppDef, error) {
	var db map[string]AppDef
	if err := yaml.Unmarshal(appsYAML, &db); err != nil {
		return nil, err
	}
	return db, nil
}
