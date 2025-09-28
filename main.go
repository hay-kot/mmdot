package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/hay-kot/mmdot/internal/commands"
	"github.com/hay-kot/mmdot/internal/core"
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
	flags := &core.Flags{}

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	app := &cli.Command{
		EnableShellCompletion: true,
		Name:                  "mmdot",
		Usage:                 `A tiny and terrible dotfiles utility for managing my machines. Probably don't use this.`,
		Version:               build(),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "log-level",
				Aliases:     []string{"l"},
				Usage:       "set the logging verbosity level",
				Value:       "info",
				Sources:     envvars("LOG_LEVEL"),
				Destination: &flags.LogLevel,
			},
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "path to the mmdot configuration file",
				Required:    false,
				Value:       "mmdot.toml",
				Sources:     envvars("CONFIG_PATH"),
				Destination: &flags.ConfigFilePath,
			},
		},
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			level, err := zerolog.ParseLevel(c.String("log-level"))
			if err != nil {
				return ctx, fmt.Errorf("failed to parse log level: %w", err)
			}

			log.Logger = log.Level(level)

			log.Debug().
				Str("log-level", flags.LogLevel).
				Str("config", flags.ConfigFilePath).
				Msg("global flags")

			return ctx, nil
		},
	}

	subcommands := []subcommand{
		commands.NewScriptsCmd(flags),
		commands.NewBrewCmd(flags),
	}

	for _, s := range subcommands {
		app = s.Register(app)
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal().Err(err).Msg("failed to run mmdot")
	}
}

// envars adds a namespace prefix for the environment variables of the application
func envvars(strs ...string) cli.ValueSourceChain {
	withPrefix := make([]string, len(strs))

	for i, str := range strs {
		withPrefix[i] = core.EnvPrefix + str
	}

	return cli.EnvVars(withPrefix...)
}

type subcommand interface {
	Register(*cli.Command) *cli.Command
}
