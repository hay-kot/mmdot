package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"text/template"

	"filippo.io/age"
	"github.com/BurntSushi/toml"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/internal/generator"
	"github.com/hay-kot/mmdot/pkgs/fcrypt"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

type GenerateCmd struct {
	coreFlags *core.Flags
}

func NewGenerateCmd(coreFlags *core.Flags) *GenerateCmd {
	return &GenerateCmd{coreFlags: coreFlags}
}

func (gc *GenerateCmd) Register(app *cli.Command) *cli.Command {
	cmd := &cli.Command{
		Name:  "generate",
		Usage: "Generate files from templates",
		Description: `Renders template files using the configured template jobs.

Templates are configured in the mmdot.toml file under the [templates] section.
Each job specifies a template file and output location, with optional variables.
All paths are relative to the configuration file directory.

Variables can be loaded from encrypted files using the varsFile option. Encrypted
files must have a .age extension and will be decrypted using your configured age
identity. The decrypted content should be in TOML format.

Example configuration:
  [templates]
  jobs = [
    { template = "templates/gitconfig.tmpl", output = "configs/.gitconfig", vars = { email = "user@example.com" } },
    { template = "templates/ssh_config.tmpl", output = "configs/.ssh/config", vars_file = "vars/secrets.toml" }
  ]

  [templates.vars]
  user = "myusername"`,
		Action: gc.generate,
	}

	app.Commands = append(app.Commands, cmd)
	return app
}

func (gc *GenerateCmd) generate(ctx context.Context, c *cli.Command) error {
	cfg, err := setupEnv(gc.coreFlags.ConfigFilePath)
	if err != nil {
		return err
	}

	if len(cfg.Templates.Jobs) == 0 {
		log.Info().Msg("No templates configured")
		return nil
	}

	for _, job := range cfg.Templates.Jobs {
		tmplContent, err := os.ReadFile(job.Template)
		if err != nil {
			return fmt.Errorf("failed to read template file %s: %w", job.Template, err)
		}

		tmpl := template.New(filepath.Base(job.Template))

		// Apply strict mode if enabled
		if cfg.Templates.StrictMode {
			tmpl = tmpl.Option("missingkey=error")
		}

		tmpl, err = tmpl.Parse(string(tmplContent))
		if err != nil {
			return generator.NewTemplateError(job.Template, err)
		}

		// Load Variables
		var fileVars map[string]any
		if job.VarsFile != "" {
			identity, err := cfg.Age.ReadIdentity()
			if err != nil {
				log.Warn().Err(err).Msg("failed to load identity file")
			}

			log.Debug().Str("path", job.VarsFile).Msg("loading vars file")

			fileVars, err = gc.loadVarsFile(job.VarsFile, identity)
			if err != nil {
				return fmt.Errorf("failed to load vars file %s: %w", job.VarsFile, err)
			}

			log.Debug().Interface("fileVars", fileVars).Msg("file vars")
		}

		// Merge vars in order: global < file < job-specific
		// The MergeVars already handles global + job-specific,
		// but we need to insert fileVars in between
		vars := map[string]any{}
		maps.Copy(vars, cfg.Templates.Vars)
		maps.Copy(vars, fileVars)
		maps.Copy(vars, job.Vars)

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, vars); err != nil {
			return generator.NewTemplateError(job.Template, err)
		}

		if err := os.MkdirAll(filepath.Dir(job.Output), 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		if err := os.WriteFile(job.Output, buf.Bytes(), 0o644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}

	}

	return nil
}

func (gc *GenerateCmd) loadVarsFile(path string, identity age.Identity) (map[string]any, error) {
	path = path + ".age" // TODO: support unencrypted files or encrypted ones
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	buff := bytes.NewBuffer([]byte{})

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	err = fcrypt.DecryptReader(file, buff, identity)
	if err != nil {
		return nil, err
	}

	vars := map[string]any{}
	_, err = toml.Decode(buff.String(), &vars)
	if err != nil {
		return nil, err
	}

	return vars, nil
}
