package appdata

import (
	_ "embed"
	"slices"

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
	Tags     []string `yaml:"tags,omitempty"`
	Files    []string `yaml:"files,omitempty"`
	XDGFiles []string `yaml:"xdg_files,omitempty"`
}

// AppGroup maps a set of tags to a list of app IDs.
type AppGroup struct {
	Tags []string `yaml:"tags"`
	IDs  []string `yaml:"ids"`
}

// TaggedApp pairs an AppDef with accumulated tags.
type TaggedApp struct {
	AppDef
	Tags []string
}

// BuildAppDB loads the embedded DB, applies the app groups and ignore list,
// then merges custom entries. Apps accumulate tags from all groups they appear in.
func BuildAppDB(groups []AppGroup, ignore []string, custom []CustomEntry) (map[string]TaggedApp, error) {
	db, err := LoadAppDB()
	if err != nil {
		return nil, err
	}

	result := make(map[string]TaggedApp)

	if len(groups) > 0 {
		// Build from groups — each app accumulates tags from all groups it belongs to
		for _, g := range groups {
			for _, id := range g.IDs {
				if def, ok := db[id]; ok {
					existing, exists := result[id]
					if exists {
						existing.Tags = appendUnique(existing.Tags, g.Tags...)
						result[id] = existing
					} else {
						result[id] = TaggedApp{
							AppDef: def,
							Tags:   slices.Clone(g.Tags),
						}
					}
				}
			}
		}
	} else {
		// No groups defined — include all apps without tags
		for id, def := range db {
			result[id] = TaggedApp{AppDef: def}
		}
	}

	for _, id := range ignore {
		delete(result, id)
	}

	for _, c := range custom {
		result[c.ID] = TaggedApp{
			AppDef: AppDef{
				Name:     c.Name,
				Files:    c.Files,
				XDGFiles: c.XDGFiles,
			},
			Tags: slices.Clone(c.Tags),
		}
	}

	return result, nil
}

// AllIDs returns a flat deduplicated list of all app IDs from the groups.
func AllIDs(groups []AppGroup) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, g := range groups {
		for _, id := range g.IDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// appendUnique appends values to a slice, skipping duplicates.
func appendUnique(base []string, vals ...string) []string {
	seen := make(map[string]bool, len(base))
	for _, v := range base {
		seen[v] = true
	}
	for _, v := range vals {
		if !seen[v] {
			base = append(base, v)
			seen[v] = true
		}
	}
	return base
}

// LoadAllApps is a convenience for loading the full embedded DB as TaggedApps (no tags).
func LoadAllApps() (map[string]TaggedApp, error) {
	db, err := LoadAppDB()
	if err != nil {
		return nil, err
	}
	result := make(map[string]TaggedApp, len(db))
	for id, def := range db {
		result[id] = TaggedApp{AppDef: def}
	}
	return result, nil
}

