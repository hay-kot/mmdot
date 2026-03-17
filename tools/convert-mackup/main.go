package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

type AppDef struct {
	Name     string   `yaml:"name"`
	Files    []string `yaml:"files,omitempty"`
	XDGFiles []string `yaml:"xdg_files,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	srcDir := "mackup/src/mackup/applications"
	outFile := "internal/appdata/apps.yml"

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("reading source dir: %w", err)
	}

	apps := make(map[string]AppDef)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".cfg") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".cfg")
		id = strings.ToLower(id)

		app, err := parseCfg(filepath.Join(srcDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}

		apps[id] = app
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(apps))
	for k := range apps {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ordered := make(yaml.MapSlice, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, yaml.MapItem{Key: k, Value: apps[k]})
	}

	out, err := yaml.Marshal(ordered)
	if err != nil {
		return fmt.Errorf("marshaling yaml: %w", err)
	}

	if err := os.WriteFile(outFile, out, 0644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	fmt.Printf("wrote %d apps to %s\n", len(apps), outFile)
	return nil
}

func parseCfg(path string) (AppDef, error) {
	f, err := os.Open(path)
	if err != nil {
		return AppDef{}, err
	}
	defer func() { _ = f.Close() }()

	var app AppDef
	var section string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}

		switch section {
		case "application":
			if strings.HasPrefix(line, "name") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					app.Name = strings.TrimSpace(parts[1])
				}
			}
		case "configuration_files":
			app.Files = append(app.Files, line)
		case "xdg_configuration_files":
			app.XDGFiles = append(app.XDGFiles, line)
		}
	}

	return app, scanner.Err()
}
