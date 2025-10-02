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
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/pkgs/printer"
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
		Usage:     "Execute scripts from the mmdot.yaml configuration",
		ArgsUsage: "[group]",
		Description: `Execute scripts defined in your mmdot.yaml configuration file.
 Scripts can be run by specifying a group (which resolves to tags), filtering by tags,
 or through interactive selection.

 Examples:
	 mmdot run personal        # Run all scripts with tags from 'personal' group
	 mmdot run --tags work     # Run all scripts tagged with 'work'
	 mmdot run --list personal # List scripts in 'personal' without executing
	 mmdot run                 # Interactive script selection`,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:        "tags",
				Aliases:     []string{"t"},
				Usage:       "filter scripts by tags (can specify multiple)",
				Destination: &sc.flags.Tags,
			},
			&cli.BoolFlag{
				Name:        "list",
				Aliases:     []string{"ls", "l"},
				Usage:       "list matching scripts without executing them",
				Destination: &sc.flags.List,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "group",
				UsageText:   "group name to be applied to arguments",
				Min:         0,
				Max:         1,
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &sc.group,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			cfg, err := core.SetupEnv(sc.coreFlags.ConfigFilePath)
			if err != nil {
				return err
			}

			log.Debug().
				Strs("tags", sc.flags.Tags).
				Bool("list", sc.flags.List).
				Str("args:group", sc.group).
				Msg("run cmd")

			return sc.run(ctx, cfg)
		},
	}

	app.Commands = append(app.Commands, cmd)
	return app
}

func (sc *RunCmd) run(ctx context.Context, cfg core.ConfigFile) error {
	// Get terminal width
	terminalWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		// Fallback to a default width if unable to get terminal size
		terminalWidth = 80
	}

	// Gather scripts based on selection mode
	var matchedScripts []core.Script
	var tagsToFilter []string

	switch {
	case sc.group != "":
		// Get tags for the specified group
		group, exists := cfg.Groups[sc.group]
		if !exists {
			return fmt.Errorf("group '%s' not found in configuration", sc.group)
		}
		tagsToFilter = group.Tags

		// Apply additional tag filtering if tags are specified via flags
		if len(sc.flags.Tags) > 0 {
			tagsToFilter = append(tagsToFilter, sc.flags.Tags...)
		}

		// Filter scripts by tags
		matchedScripts = filterScriptsByTags(cfg.Exec.Scripts, tagsToFilter)

	case len(sc.flags.Tags) > 0:
		// Filter by tags from flags
		matchedScripts = filterScriptsByTags(cfg.Exec.Scripts, sc.flags.Tags)

	default:
		// Interactive selection mode
		options := []huh.Option[string]{}
		scriptMap := make(map[string]core.Script)

		for _, s := range cfg.Exec.Scripts {
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
		p := printer.New(os.Stdout)
		p.LineBreak()
		var items []string
		for _, script := range matchedScripts {
			items = append(items, fmt.Sprintf("%s (tags: %s)", script.Path, strings.Join(script.Tags, ", ")))
		}
		p.List("Scripts to run:", items)
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
		if err := os.Chmod(script.Path, 0o755); err != nil {
			log.Error().Err(err).Str("path", script.Path).Msg("Failed to set script permissions")
			return err
		}

		// Execute script with the configured shell
		cmd := exec.CommandContext(scriptCtx, cfg.Exec.Shell, script.Path)
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

// filterScriptsByTags returns scripts that match all specified tags
func filterScriptsByTags(scripts []core.Script, tags []string) []core.Script {
	if len(tags) == 0 {
		return scripts
	}

	var filtered []core.Script

	for _, script := range scripts {
		// Check if script has all the required tags
		hasAllTags := true
		for _, requiredTag := range tags {
			found := false
			for _, scriptTag := range script.Tags {
				if scriptTag == requiredTag {
					found = true
					break
				}
			}
			if !found {
				hasAllTags = false
				break
			}
		}

		if hasAllTags {
			filtered = append(filtered, script)
		}
	}

	return filtered
}
