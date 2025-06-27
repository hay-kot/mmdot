package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/hay-kot/mmdot/internal/actions"
	"github.com/hay-kot/mmdot/internal/brew"
	"github.com/hay-kot/mmdot/internal/commands"
)

var (
	// Build information. Populated at build-time via -ldflags flag.
	version = "dev"
	commit  = "HEAD"
	date    = time.Now().Format(time.DateTime)
)

func build() string {
	short := commit
	if len(commit) > 7 {
		short = commit[:7]
	}

	return fmt.Sprintf("%s (%s) %s", version, short, date)
}

func main() {
	ctrl := &commands.Controller{
		Flags: &commands.Flags{},
	}

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	app := &cli.Command{
		EnableShellCompletion: true,
		Name:                  "mmdot",
		Usage:                 `A tiny and terrible dotfiles utility for managing my machines. Probably don't use this.`,
		Version:               build(),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "log level (debug, info, warn, error, fatal, panic)",
				Value:   "info",
				Sources: cli.EnvVars("MMDOT_LOG_LEVEL"),
			},
			&cli.StringFlag{
				Name:     "config",
				Usage:    "config file path",
				Required: false,
				Value:    "mmdot.toml",
				Sources:  cli.EnvVars("MMDOT_CONFIG_PATH"),
			},
		},
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			level, err := zerolog.ParseLevel(c.String("log-level"))
			if err != nil {
				return ctx, fmt.Errorf("failed to parse log level: %w", err)
			}

			log.Logger = log.Level(level)

			return ctx, nil
		},
		Commands: []*cli.Command{
			{
				Name:      "run",
				Usage:     "runs scripts from the mmdot.toml file",
				ArgsUsage: "tags for scripts to run",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "tags",
						Usage:    "tags to run",
						Required: false,
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					cfgpath := c.String("config")

					cfg, err := setupEnv(cfgpath)
					if err != nil {
						return err
					}

					flags := commands.FlagsRun{
						Tags:   c.StringSlice("tags"),
						Action: c.Args().First(),
					}

					return ctrl.Run(ctx, cfg.Exec, cfg.Bundles, cfg.Actions, flags)
				},
			},
			{
				Name: "diff",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "display-included",
						Usage: "display brews that are on the machine and config",
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					cfgpath := c.String("config")
					cfg, err := setupEnv(cfgpath)
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
					if c.Bool("display-included") {
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
				},
			},
			{
				Name: "compile",
				Action: func(ctx context.Context, c *cli.Command) error {
					cfgpath := c.String("config")

					cfg, err := setupEnv(cfgpath)
					if err != nil {
						return err
					}

					for v := range maps.Keys(cfg.Brews) {
						cfg := brew.Get(cfg.Brews, v)

						if cfg.Outfile == "" {
							continue
						}

						// Create directory
						err := os.MkdirAll(filepath.Dir(cfg.Outfile), 0755)
						if err != nil {
							return err
						}

						err = os.WriteFile(cfg.Outfile, []byte(cfg.String()), 0644)
						if err != nil {
							return err
						}

						log.Info().Str("file", cfg.Outfile).Msg("outfile written")
					}

					return nil
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal().Err(err).Msg("failed to run mmdot")
	}
}

func setupEnv(cfgpath string) (ConfigFile, error) {
	cfg := ConfigFile{
		Exec:    actions.ExecConfig{},
		Brews:   map[string]*brew.Config{},
		Bundles: map[string]actions.Bundle{},
		Actions: map[string]actions.Action{},
	}
	absolutePath, err := filepath.Abs(cfgpath)
	if err != nil {
		return cfg, err
	}

	err = os.Chdir(filepath.Dir(absolutePath))
	if err != nil {
		return cfg, err
	}

	log.Debug().Str("cwd", filepath.Dir(absolutePath)).Msg("setting working directory to config dir")

	_, err = toml.DecodeFile(cfgpath, &cfg)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}
