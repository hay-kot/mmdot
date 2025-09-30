package commands

import (
	"context"

	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/internal/generator"
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
		Description: `Renders template files using the configured templates.

Templates are configured in the mmdot.yml file under the templates section.
Each template specifies inline template content and output location, with optional variables.
All paths are relative to the configuration file directory.

Variables can be loaded from encrypted files using var_files. Encrypted
files should have a ?vault=true query parameter and will be decrypted using your
configured age identity. The decrypted content should be in TOML format.

Example configuration:
  variables:
    vars:
      user: myusername
    var_files:
      - ./.data/vars/secrets.toml?vault=true
      - ./.data/vars/plain.toml

  templates:
    - name: SSH Config
      template: |
        Host {{ .hostname }}
          User {{ .user }}
      output: ~/.ssh/config
      perm: 0600
      vars:
        hostname: example.com`,
		Action: gc.generate,
	}

	app.Commands = append(app.Commands, cmd)
	return app
}

func (gc *GenerateCmd) generate(ctx context.Context, c *cli.Command) error {
	cfg, err := core.SetupEnv(gc.coreFlags.ConfigFilePath)
	if err != nil {
		return err
	}

	engine := generator.NewEngine(&cfg)

	for _, tmpl := range cfg.Templates {
		if err := engine.RenderTemplate(ctx, tmpl); err != nil {
			return err
		}
	}

	return nil
}
