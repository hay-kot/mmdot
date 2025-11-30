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
	"github.com/hay-kot/mmdot/pkgs/cll"
	"github.com/hay-kot/mmdot/pkgs/printer"
)

var (
	// Build information. Populated at build-time via -ldflags flag.
	version = "v0.3.0-develop"
	commit  = "HEAD"
	date    = time.Now().Format(time.DateTime)
)

var envvars = cll.EnvWithPrefix(core.EnvPrefix)

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

	var (
		ctx    = context.Background()
		writer = printer.NewDeferedWriter(os.Stdout)
	)

	ctx = printer.WithWriter(ctx, writer)
	printer.ConsolePrinter = printer.Ctx(ctx)

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
				Value:       "mmdot.yml",
				Sources:     envvars("CONFIG_PATH"),
				Destination: &flags.ConfigFilePath,
			},
		},
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			level, err := zerolog.ParseLevel(flags.LogLevel)
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
		OnUsageError: func(ctx context.Context, cmd *cli.Command, err error, isSubcommand bool) error {
			return err
		},
	}

	app = cll.Register(app,
		commands.NewScriptsCmd(flags),
		commands.NewBrewCmd(flags),
		commands.NewEncryptCmd(flags),
		commands.NewHookCmd(flags),
	)

	exitCode := 0
	if err := app.Run(context.Background(), os.Args); err != nil {
		printer.Ctx(ctx).FatalError(err)
		exitCode = 1
	}

	err := writer.Flush()
	if err != nil {
		panic(err)
	}
	os.Exit(exitCode)
}
