package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/charmbracelet/huh"
	"github.com/hay-kot/mmdot/internal/actions"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

type RunCmd struct {
	coreFlags *core.Flags
	flags     struct {
		Tags []string
		List bool
	}
	group string
}

func NewScriptsCmd(coreFlags *core.Flags) *RunCmd {
	return &RunCmd{
		coreFlags: coreFlags,
	}
}

func (sc *RunCmd) Register(app *cli.Command) *cli.Command {
	cmd := &cli.Command{
		Name:      "run",
		Usage:     "runs scripts from the mmdot.toml file",
		ArgsUsage: "the group of scripts to run",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:        "tags",
				Usage:       "tags to run",
				Destination: &sc.flags.Tags,
			},
			&cli.BoolFlag{
				Name:        "list",
				Aliases:     []string{"ls"},
				Usage:       "list groups scripts without running them",
				Destination: &sc.flags.List,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "group",
				UsageText:   "the group of scripts to run",
				Min:         1,
				Max:         1,
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &sc.group,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			cfg, err := setupEnv(sc.coreFlags.ConfigFilePath)
			if err != nil {
				return err
			}

			log.Debug().
				Strs("tags", sc.flags.Tags).
				Bool("list", sc.flags.List).
				Str("args:group", sc.group).
				Msg("run cmd")

			return sc.run(ctx, cfg.Exec, cfg.Bundles, cfg.Actions)
		},
	}

	app.Commands = append(app.Commands, cmd)
	return app
}

func (sc *RunCmd) run(
	ctx context.Context,
	execs actions.ExecConfig,
	bundles map[string]actions.Bundle,
	actionsMap map[string]actions.Action,
) error {
	// Get terminal width
	terminalWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		// Fallback to a default width if unable to get terminal size
		terminalWidth = 80
	}

	// Gather scripts based on selection mode
	var matchedScripts []actions.Script

	switch {
	case sc.group != "":
		// Get scripts for a specific action
		actionScripts, err := actions.GetScriptsForAction(actionsMap, bundles, sc.group)
		if err != nil {
			return err
		}

		// Apply tag filtering if tags are specified
		if len(sc.flags.Tags) > 0 {
			matchedScripts = actions.FilterScriptsByTags(actionScripts, sc.flags.Tags)
		} else {
			matchedScripts = actionScripts
		}

	case len(sc.flags.Tags) > 0:
		// Collect all scripts from all bundles
		var allScripts []actions.Script
		for _, bundle := range bundles {
			allScripts = append(allScripts, bundle.Scripts...)
		}

		// Filter by tags
		matchedScripts = actions.FilterScriptsByTags(allScripts, sc.flags.Tags)

	default:
		// Interactive selection mode - go straight to individual scripts selection
		var allScripts []actions.Script

		// Collect all scripts from all bundles
		for _, bundle := range bundles {
			allScripts = append(allScripts, bundle.Scripts...)
		}

		options := []huh.Option[string]{}
		scriptMap := make(map[string]actions.Script)

		for _, s := range allScripts {
			displayStr := fmt.Sprintf("%s (%s)", s.Path, strings.Join(s.Tags, ", "))
			options = append(options, huh.NewOption(displayStr, s.Path))
			scriptMap[s.Path] = s
		}

		selected := []string{}
		err := huh.NewMultiSelect[string]().
			Title("Select Scripts to Run").
			Options(options...).
			Value(&selected).
			Run()

		if err != nil {
			return err
		}

		for _, selectedPath := range selected {
			if script, ok := scriptMap[selectedPath]; ok {
				matchedScripts = append(matchedScripts, script)
			}
		}
	}

	if len(matchedScripts) == 0 {
		fmt.Println("No scripts matched the specified criteria")
		return nil
	}

	// If list flag is set, just list the scripts without executing
	if sc.flags.List {
		fmt.Println("Scripts to run:")
		for _, script := range matchedScripts {
			fmt.Printf("  %s (tags: %s)\n", script.Path, strings.Join(script.Tags, ", "))
		}
		return nil
	}

	// Create a cancellation context with signal handling
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Execute matched scripts
	for _, script := range matchedScripts {
		// Create dividing line
		dividerPrefix := fmt.Sprintf("-- [SCRIPT] %s ", filepath.Base(script.Path))
		dividerRemainder := strings.Repeat("-", terminalWidth-len(dividerPrefix))

		// Create a cancelable context for each script
		scriptCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		fmt.Println(dividerPrefix + dividerRemainder)
		log.Debug().Str("path", script.Path).Strs("tags", script.Tags).Msg("Executing script")

		// Make script executable
		if err := os.Chmod(script.Path, 0755); err != nil {
			log.Error().Err(err).Str("path", script.Path).Msg("Failed to set script permissions")
			return err
		}

		// Execute script with the configured shell
		cmd := exec.CommandContext(scriptCtx, execs.Shell, script.Path)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		if err := cmd.Run(); err != nil {
			log.Error().Err(err).Str("path", script.Path).Msg("Script execution failed")
			return err
		}

		// Add a newline after script execution for readability
		fmt.Println()
	}
	return nil
}
