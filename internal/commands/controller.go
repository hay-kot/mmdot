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
	"github.com/hay-kot/mmdot/internal/core"
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
	All  bool
	Tags []string
}

func (c *Controller) Run(ctx context.Context, execs core.Exec, flags FlagsRun) error {
	// Get terminal width
	terminalWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		// Fallback to a default width if unable to get terminal size
		terminalWidth = 80
	}

	// Filter scripts based on tags
	var matchedScripts []core.Script

	switch {
	case flags.All:
		matchedScripts = execs.Scripts
	case len(flags.Tags) == 0:
		options := []huh.Option[string]{}

		for _, s := range execs.Scripts {
			str := fmt.Sprintf("%s, (%s)", s.Path, strings.Join(s.Tags, ", "))
			options = append(options, huh.NewOption(str, s.Path))
		}

		selected := []string{}

		err := huh.NewMultiSelect[string]().
			Title("Scripts").
			Options(options...).
			Value(&selected).
			Run()
		if err != nil {
			return err
		}

		for _, selected := range selected {
			for _, script := range execs.Scripts {
				if script.Path == selected {
					matchedScripts = append(matchedScripts, script)
				}
			}
		}

	default:
		// Find scripts that match any of the specified tags
		for _, script := range execs.Scripts {
			if hasMatchingTag(script.Tags, flags.Tags) {
				matchedScripts = append(matchedScripts, script)
				log.Debug().Str("script", script.Path).Strs("tags", script.Tags).Msg("included")
				continue
			}
			log.Debug().Str("script", script.Path).Strs("tags", script.Tags).Msg("filtered")
		}
	}

	if len(matchedScripts) == 0 {
		fmt.Printf("no scripts matched flags: %s\n", strings.Join(flags.Tags, ", "))
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

		// Execute script with interactive I/O
		cmd := exec.CommandContext(scriptCtx, "/bin/sh", script.Path)
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

// hasMatchingTag checks if ALL requested tags are present in the script tags
func hasMatchingTag(scriptTags, requestedTags []string) bool {
	for _, reqTag := range scriptTags {
		found := false
		for _, scriptTag := range requestedTags {
			if reqTag == scriptTag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
