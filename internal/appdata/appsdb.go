package appdata

import (
	_ "embed"
	"maps"

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

// CustomEntry represents a user-defined app in mmdot.yml.
type CustomEntry struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Files    []string `yaml:"files,omitempty"`
	XDGFiles []string `yaml:"xdg_files,omitempty"`
}

// BuildAppDB loads the embedded DB, applies the app whitelist and ignore list,
// then merges custom entries. Returns the final map of app ID -> AppDef.
func BuildAppDB(apps []string, ignore []string, custom []CustomEntry) (map[string]AppDef, error) {
	db, err := LoadAppDB()
	if err != nil {
		return nil, err
	}

	result := make(map[string]AppDef)

	if len(apps) > 0 {
		for _, id := range apps {
			if def, ok := db[id]; ok {
				result[id] = def
			}
		}
	} else {
		maps.Copy(result, db)
	}

	for _, id := range ignore {
		delete(result, id)
	}

	for _, c := range custom {
		result[c.ID] = AppDef{
			Name:     c.Name,
			Files:    c.Files,
			XDGFiles: c.XDGFiles,
		}
	}

	return result, nil
}
