package generator

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"text/template"

	"filippo.io/age"
	"github.com/BurntSushi/toml"
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

	// Parse and execute template
	t := template.New(tmpl.Name)
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

	// Create output directory if needed
	if err := os.MkdirAll(filepath.Dir(tmpl.Output), 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Parse permissions
	perm := os.FileMode(0o644)
	if tmpl.Permissions != "" {
		p, err := strconv.ParseUint(tmpl.Permissions, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid permissions %s: %w", tmpl.Permissions, err)
		}
		perm = os.FileMode(p)
	}

	// Write output file
	if err := os.WriteFile(tmpl.Output, buf.Bytes(), perm); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	log.Info().
		Str("template", tmpl.Name).
		Str("output", tmpl.Output).
		Msg("rendered template")

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

	// If it's a vault file, expect .age extension
	if vf.IsVault {
		if filepath.Ext(path) != ".age" {
			path = path + ".age"
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			log.Warn().Str("path", path).Msg("vault file does not exist, skipping")
			return nil, nil
		}

		if identity == nil {
			return nil, fmt.Errorf("no identity loaded for encrypted file %s", path)
		}

		buff := bytes.NewBuffer([]byte{})
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()

		err = fcrypt.DecryptReader(file, buff, identity)
		if err != nil {
			return nil, err
		}

		vars := map[string]any{}
		if _, err = toml.Decode(buff.String(), &vars); err != nil {
			return nil, err
		}

		return vars, nil
	}

	// Non-encrypted file
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Warn().Str("path", path).Msg("vars file does not exist, skipping")
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	vars := map[string]any{}
	if _, err = toml.Decode(string(data), &vars); err != nil {
		return nil, err
	}

	return vars, nil
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
