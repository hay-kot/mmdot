package commands

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hay-kot/mmdot/internal/brew"
	"github.com/hay-kot/mmdot/internal/core"
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

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Underline(true).
		MarginTop(1)

	presentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")). // Green
		MarginLeft(2)

	absentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")). // Red
		MarginLeft(2)

	excludedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")). // Yellow
		MarginLeft(2)

	// Present items section
	if c.Bool("verbose") {
		if len(diff.Present) > 0 {
			fmt.Println(sectionStyle.Render("Present Brews:"))
			for _, item := range diff.Present {
				fmt.Println(presentStyle.Render("✓ " + item))
			}
		} else {
			fmt.Println(sectionStyle.Render("Present Brews:"))
			fmt.Println(presentStyle.Render("  None"))
		}
	}

	// Absent items section
	if len(diff.Absent) > 0 {
		fmt.Println(sectionStyle.Render("Absent Brews:"))
		for _, item := range diff.Absent {
			fmt.Println(absentStyle.Render("" + item))
		}
	}

	// Excluded items section
	if len(diff.Extra) > 0 {
		fmt.Println(sectionStyle.Render("Extra Brews:"))
		for _, item := range diff.Extra {
			fmt.Println(excludedStyle.Render("" + item))
		}
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
	fmt.Println(lipgloss.NewStyle().Italic(true).MarginTop(1).Render(summaryText))

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
