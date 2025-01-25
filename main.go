package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/hay-kot/mmdot/internal/brew"
	"github.com/hay-kot/mmdot/internal/commands"
	"github.com/hay-kot/mmdot/internal/core"
)

var (
	// Build information. Populated at build-time via -ldflags flag.
	version = "dev"
	commit  = "HEAD"
	date    = time.Now()
)

func build() string {
	short := commit
	if len(commit) > 7 {
		short = commit[:7]
	}

	return fmt.Sprintf("%s (%s) %s", version, short, date.Format(time.DateTime))
}

func main() {
	ctrl := &commands.Controller{
		Flags: &commands.Flags{},
	}

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	app := &cli.Command{
		Name:    "mmdot",
		Usage:   `A tiny and terrible dotfiles utility for managing my machines. Probably don't use this.`,
		Version: build(),
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
					&cli.BoolFlag{
						Name:     "all",
						Usage:    "run all scripts",
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
						All:  c.Bool("all"),
						Tags: c.Args().Slice(),
					}

					return ctrl.Run(ctx, cfg.Exec, flags)
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
		Exec:  core.Exec{},
		Brews: map[string]*brew.Config{},
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
