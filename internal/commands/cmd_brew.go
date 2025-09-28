package commands

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hay-kot/mmdot/internal/brew"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/pkgs/printer"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

type BrewCmd struct {
	flags *core.Flags
}

func NewBrewCmd(flags *core.Flags) *BrewCmd {
	return &BrewCmd{flags: flags}
}

func (bc *BrewCmd) Register(app *cli.Command) *cli.Command {
	cmd := &cli.Command{
		Name:  "brew",
		Usage: "Manage Homebrew packages and configurations",
		Commands: []*cli.Command{
			{
				Name:      "diff",
				Usage:     "Compare installed Homebrew packages with configuration",
				ArgsUsage: "<brew-name>",
				Description: `Compares the specified brew configuration with what's installed on the machine.
Shows absent packages (in config but not installed), extra packages (installed but not in config),
and optionally present packages (both in config and installed).

Example: mmdot brew diff personal`,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "display packages that are both in config and installed on the machine",
					},
				},
				Action: bc.diff,
			},
			{
				Name:  "compile",
				Usage: "Compile brew configurations to their output files",
				Description: `Generates Brewfile outputs for all brew configurations that have an 'outfile' specified.
This is useful for creating Brewfiles that can be used with 'brew bundle'.

The compiled files will be written to the paths specified in each brew configuration's 'outfile' field.`,
				Action: bc.compile,
			},
		},
	}

	app.Commands = append(app.Commands, cmd)
	return app
}

func (bc *BrewCmd) diff(ctx context.Context, c *cli.Command) error {
	cfg, err := setupEnv(bc.flags.ConfigFilePath)
	if err != nil {
		return err
	}
	keys := slices.Collect(maps.Keys(cfg.Brews))
	arg := c.Args().First()
	if arg == "" || !slices.Contains(keys, arg) {
		return fmt.Errorf("invalid brew, please provide one of: %v", strings.Join(keys, ", "))
	}
	brewCfg := brew.Get(cfg.Brews, arg)
	if brewCfg == nil {
		panic("brew config not found")
	}
	diff, err := brewCfg.Diff()
	if err != nil {
		return err
	}

	// Process and display results with consistent spacing
	p := printer.New(os.Stdout)
	p.LineBreak()

	// Present items section
	if c.Bool("verbose") {
		var statusItems []printer.StatusListItem
		if len(diff.Present) > 0 {
			for _, item := range diff.Present {
				statusItems = append(statusItems, printer.StatusListItem{
					Ok:     true,
					Status: item,
				})
			}
		} else {
			statusItems = append(statusItems, printer.StatusListItem{
				Ok:     true,
				Status: "None",
			})
		}
		p.StatusList("Present Brews:", statusItems)
		p.LineBreak()
	}

	// Absent items section
	if len(diff.Absent) > 0 {
		var statusItems []printer.StatusListItem
		for _, item := range diff.Absent {
			statusItems = append(statusItems, printer.StatusListItem{
				Ok:     false,
				Status: item,
			})
		}
		p.StatusList("Absent Brews:", statusItems)
		p.LineBreak()
	}

	// Extra items section
	if len(diff.Extra) > 0 {
		var items []string
		for _, item := range diff.Extra {
			items = append(items, item)
		}
		p.List("Extra Brews:", items)
		p.LineBreak()
	}

	// Display summary
	totalConfig := len(diff.Present) + len(diff.Absent) + len(diff.Extra)
	summaryText := fmt.Sprintf(
		"Summary: %d brews in config (%d present, %d absent, %d excluded)",
		totalConfig,
		len(diff.Present),
		len(diff.Absent),
		len(diff.Extra),
	)
	fmt.Println(summaryText)

	return nil
}

func (bc *BrewCmd) compile(ctx context.Context, c *cli.Command) error {
	cfg, err := setupEnv(bc.flags.ConfigFilePath)
	if err != nil {
		return err
	}

	for v := range cfg.Brews {
		cfg := brew.Get(cfg.Brews, v)

		if cfg.Outfile == "" {
			continue
		}

		// Create directory
		err := os.MkdirAll(filepath.Dir(cfg.Outfile), 0o755)
		if err != nil {
			return err
		}

		err = os.WriteFile(cfg.Outfile, []byte(cfg.String()), 0o644)
		if err != nil {
			return err
		}

		log.Info().Str("file", cfg.Outfile).Msg("outfile written")
	}

	return nil
}
