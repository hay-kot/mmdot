// Package commands contains the CLI commands for the application
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
	"github.com/rs/zerolog/log"
	"golang.org/x/term"
)

type Flags struct {
	LogLevel string
}

type Controller struct {
	Flags *Flags
}

type FlagsRun struct {
	Tags   []string
	Action string
}

func (c *Controller) Run(
	ctx context.Context,
	execs actions.ExecConfig,
	bundles map[string]actions.Bundle,
	actionsMap map[string]actions.Action,
	flags FlagsRun,
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
	case flags.Action != "":
		// Get scripts for a specific action
		actionScripts, err := actions.GetScriptsForAction(actionsMap, bundles, flags.Action)
		if err != nil {
			return err
		}
		
		// Apply tag filtering if tags are specified
		if len(flags.Tags) > 0 {
			matchedScripts = actions.FilterScriptsByTags(actionScripts, flags.Tags)
		} else {
			matchedScripts = actionScripts
		}

	case len(flags.Tags) > 0:
		// Collect all scripts from all bundles
		var allScripts []actions.Script
		for _, bundle := range bundles {
			allScripts = append(allScripts, bundle.Scripts...)
		}
		
		// Filter by tags
		matchedScripts = actions.FilterScriptsByTags(allScripts, flags.Tags)

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
