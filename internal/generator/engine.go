package generator

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"text/template"

	"filippo.io/age"
	"github.com/goccy/go-yaml"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/pkgs/fcrypt"
	"github.com/rs/zerolog/log"
)

type Engine struct {
	cfg *core.ConfigFile

	globalVars map[string]any
	fileVars   map[string]any
}

func NewEngine(cfg *core.ConfigFile) *Engine {
	return &Engine{
		cfg:        cfg,
		globalVars: make(map[string]any),
		fileVars:   make(map[string]any),
	}
}

func (e *Engine) RenderTemplate(ctx context.Context, tmpl core.Template) error {
	// Preload variables if not already done
	if len(e.globalVars) == 0 && len(e.fileVars) == 0 {
		if err := e.preloadVars(); err != nil {
			return fmt.Errorf("failed to preload vars: %w", err)
		}
	}

	// Parse built-in partials, then the user's template
	t := template.New(tmpl.Name).Funcs(e.funcMap())
	for name, body := range builtinPartials {
		if _, err := t.New(name).Parse(body); err != nil {
			return fmt.Errorf("failed to parse builtin partial %q: %w", name, err)
		}
	}
	t, err := t.Parse(tmpl.Template)
	if err != nil {
		return NewTemplateError(tmpl.Name, err)
	}

	// Merge variables: global < file < template-specific
	vars := MergeMaps(e.globalVars, e.fileVars, tmpl.Vars)

	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return NewTemplateError(tmpl.Name, err)
	}

	// Get output bytes
	output := buf.Bytes()

	// Trim leading and trailing whitespace if requested
	if tmpl.ShouldTrim() {
		output = bytes.TrimSpace(output)
	}

	// Create output directory if needed
	if err := os.MkdirAll(filepath.Dir(tmpl.Output), 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Parse permissions
	perm := os.FileMode(0o644)
	if tmpl.Permissions != "" {
		p, err := core.ParseOctalPermissions(tmpl.Permissions)
		if err != nil {
			return fmt.Errorf("invalid permissions %s: %w", tmpl.Permissions, err)
		}
		perm = p
	}

	// Write output file
	if err := os.WriteFile(tmpl.Output, output, perm); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	return nil
}

// preloadVars loads variables from the [core.ConfigFile] based on the var files
// this sets the globalVars and fileVars properties and should be called before
// rendering a template.
func (e *Engine) preloadVars() error {
	// Load global vars
	e.globalVars = e.cfg.Variables.Vars

	// Load identity for encrypted files
	var identity age.Identity
	var err error
	if e.cfg.Age.IdentityFile != "" {
		identity, err = e.cfg.Age.ReadIdentity()
		if err != nil {
			log.Warn().Err(err).Msg("failed to load identity file")
		}
	}

	// Load variable files
	for _, vf := range e.cfg.Variables.VarFiles {
		vars, err := e.loadVarsFile(vf, identity)
		if err != nil {
			return fmt.Errorf("failed to load vars file %s: %w", vf.Path, err)
		}

		// Merge into fileVars
		maps.Copy(e.fileVars, vars)
	}

	return nil
}

func (e *Engine) loadVarsFile(vf core.VarFile, identity age.Identity) (map[string]any, error) {
	path := vf.Path

	// If it's a vault file, try encrypted version first, then fall back to unencrypted
	if vf.IsVault {
		encryptedPath := path
		if filepath.Ext(path) != ".age" {
			encryptedPath = path + ".age"
		}

		// Try encrypted file first
		if _, err := os.Stat(encryptedPath); err == nil {
			if identity == nil {
				return nil, fmt.Errorf("no identity loaded for encrypted file %s", encryptedPath)
			}

			buff := bytes.NewBuffer([]byte{})
			file, err := os.Open(encryptedPath)
			if err != nil {
				return nil, err
			}
			defer func() { _ = file.Close() }()

			err = fcrypt.DecryptReader(file, buff, identity)
			if err != nil {
				return nil, err
			}

			vars := map[string]any{}
			if err = yaml.Unmarshal(buff.Bytes(), &vars); err != nil {
				return nil, err
			}

			return vars, nil
		}

		// Fall back to unencrypted file
		log.Debug().Str("encrypted_path", encryptedPath).Str("fallback_path", path).Msg("encrypted vault not found, trying unencrypted")
	}

	// Non-encrypted file (or vault fallback)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Warn().Str("path", path).Msg("vars file does not exist, skipping")
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	vars := map[string]any{}
	if err = yaml.Unmarshal(data, &vars); err != nil {
		return nil, err
	}

	return vars, nil
}

// builtinPartials are named templates automatically available in all user templates.
// Invoke with {{template "name" arg}}.
var builtinPartials = map[string]string{
	// brewfile renders brew tap/install/uninstall commands for a named brew config.
	// Usage: {{template "brewfile" "personal"}}
	// The argument is the brew config key from the brews: section in mmdot.yml.
	"brewfile": `
{{- $b := brewConfig . -}}
{{- $tap := "tap" -}}{{- $install := "install" -}}
{{- if $b.Remove -}}{{- $tap = "untap" -}}{{- $install = "uninstall" -}}{{- end -}}
{{- if $b.Taps -}}
# Homebrew Taps
{{range $b.Taps -}}
brew {{$tap}} {{.}}
{{end}}
{{end -}}
{{- if $b.Brews -}}
# Homebrew Packages
{{range $b.Brews -}}
brew {{$install}} {{.}}
{{end}}
{{end -}}
{{- if $b.Casks -}}
# Homebrew Casks
{{range $b.Casks -}}
brew {{$install}} --cask {{.}}
{{end}}
{{end -}}
{{- if $b.MAS -}}
# Mac App Store
{{range $b.MAS -}}
mas install {{.}}
{{end}}
{{end -}}`,
}

// funcMap returns template functions available to all templates.
func (e *Engine) funcMap() template.FuncMap {
	return template.FuncMap{
		// brewConfig resolves a named brew configuration (with includes merged)
		// and returns it for use in templates.
		//
		// Usage: {{$b := brewConfig "personal"}}
		//        {{range $b.Taps}}brew tap {{.}}{{end}}
		"brewConfig": func(name string) (*core.Brews, error) {
			b := e.cfg.Brews.Get(name)
			if b == nil {
				return nil, fmt.Errorf("brew config %q not found", name)
			}
			return b, nil
		},
	}
}

// MergeMaps merges multiple maps with later maps taking precedence over earlier ones.
// Returns a new map without modifying the input maps.
func MergeMaps[K comparable, V any](mps ...map[K]V) map[K]V {
	result := make(map[K]V)

	for _, m := range mps {
		maps.Copy(result, m)
	}

	return result
}
