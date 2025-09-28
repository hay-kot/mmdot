package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/internal/generator"
	"github.com/hay-kot/mmdot/pkgs/printer"
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

Example configuration:
  [templates]
  jobs = [
    { template = "templates/gitconfig.tmpl", output = "configs/.gitconfig", vars = { email = "user@example.com" } }
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

	gen := generator.New(&cfg.Templates)
	if err := gen.Generate(); err != nil {
		// Check if it's a template error with formatted output
		if te, ok := err.(*generator.TemplateError); ok {
			p := printer.New(os.Stderr)
			p.LineBreak()
			p.FatalError(te)
			return fmt.Errorf("template generation failed")
		}
		return err
	}

	return nil
}
