package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

type RunCmd struct {
	coreFlags *core.Flags
	flags     struct {
		Types []string
		List  bool
	}
	expr string
}

func NewScriptsCmd(coreFlags *core.Flags) *RunCmd {
	return &RunCmd{
		coreFlags: coreFlags,
	}
}

func (sc *RunCmd) Register(app *cli.Command) *cli.Command {
	cmd := &cli.Command{
		Name:      "run",
		Usage:     "Execute scripts and generate templates from the mmdot.yaml configuration",
		ArgsUsage: "[expression]",
		Description: `Execute scripts and generate templates defined in your mmdot.yaml configuration file.
 Items can be filtered using expressions or selected interactively.

 Examples:
	 mmdot run                                    # Interactive selection
	 mmdot run "true"                             # Run all templates and scripts
	 mmdot run '"work" in tags'                   # Run items tagged with 'work'
	 mmdot run 'name == "mytemplate"'             # Run specific item by name
	 mmdot run --type template                    # Generate all templates
	 mmdot run --type script '"deploy" in tags'   # Run scripts tagged with 'deploy'
	 mmdot run --list '"prod" in tags'            # List items without executing

 Expression variables:
	 - name: Item name (template name or script basename)
	 - path: Full path (scripts only)
	 - tags: Array of tags`,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:        "type",
				Usage:       "filter by type: 'template' or 'script' (default: both)",
				Destination: &sc.flags.Types,
				Value:       []string{RunnerTypeTemplate, RunnerTypeScript},
			},
			&cli.BoolFlag{
				Name:        "list",
				Aliases:     []string{"ls", "l"},
				Usage:       "list matching items without executing them",
				Destination: &sc.flags.List,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			cfg, err := core.SetupEnv(sc.coreFlags.ConfigFilePath)
			if err != nil {
				return err
			}

			sc.expr = strings.Join(c.Args().Slice(), " ")

			log.Debug().
				Bool("list", sc.flags.List).
				Str("expr", sc.expr).
				Strs("types", sc.flags.Types).
				Msg("run cmd")

			return sc.run(ctx, cfg)
		},
	}

	app.Commands = append(app.Commands, cmd)
	return app
}

func (sc *RunCmd) run(ctx context.Context, cfg core.ConfigFile) error {
	// TODO: Missing functionality:
	// - --list flag: runners don't expose a list-only mode yet
	types, err := RunnerTypeFromStrings(sc.flags.Types)
	if err != nil {
		return err
	}

	// Get terminal width
	terminalWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		// Fallback to a default width if unable to get terminal size
		terminalWidth = 80
	}

	// Order matters, they will be executed in the order that they are set here.
	runners := []Runner{
		NewTemplateRunner(&cfg),
		NewScriptRunner(&cfg),
	}

	// Determine execution mode: interactive vs expression-based
	useInteractiveMode := sc.expr == ""

	if useInteractiveMode {
		// Interactive selection mode
		var formGroups []*huh.Group

		for _, r := range runners {
			g := r.Form(ctx)
			if g != nil {
				formGroups = append(formGroups, g)
			}

		}

		if len(formGroups) > 0 {
			form := huh.NewForm(formGroups...)
			if err := form.Run(); err != nil {
				return err
			}
		} else {
			fmt.Println("No templates or scripts available")
			return nil
		}
	}

	// Execute args
	executeArgs := ExecuteArgs{
		Types:         types,
		TerminalWidth: terminalWidth,
		Expr:          sc.expr,
		Macros:        cfg.Macros,
	}

	for _, r := range runners {
		// Execute templates first (they may generate files that scripts need)
		if err := r.Execute(ctx, executeArgs); err != nil {
			return err
		}
	}

	return nil
}
