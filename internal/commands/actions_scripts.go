package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/charmbracelet/huh"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/rs/zerolog/log"
)

var _ Runner = &ScriptRunner{}

type ScriptRunner struct {
	cfg *core.ConfigFile

	formsActivated bool
	formsScriptMap map[string]core.Script
	formSelected   []string
}

func NewScriptRunner(cfg *core.ConfigFile) *ScriptRunner {
	return &ScriptRunner{
		cfg:            cfg,
		formsActivated: false,
		formsScriptMap: map[string]core.Script{},
		formSelected:   []string{},
	}
}

// Execute implements Runner.
func (sr *ScriptRunner) Execute(ctx context.Context, args ExecuteArgs) error {
	if !slices.Contains(args.Types, RunnerTypeScript) {
		log.Debug().Str("type", RunnerTypeScript).Msg("type disabled")
		return nil // nothing to run
	}

	scriptsToRun := []core.Script{}

	switch {
	case sr.formsActivated: // Assume form has run and we have user interactions to base selection on
		for _, selected := range sr.formSelected {
			scriptsToRun = append(scriptsToRun, sr.formsScriptMap[selected])
		}
	default:
		// Compile expression once before loop
		program, err := compileExpr(args.Expr, args.Macros)
		if err != nil {
			return fmt.Errorf("invalid expression: %w", err)
		}

		for _, script := range sr.cfg.Exec.Scripts {
			enabled, err := evalCompiledExpr(program, map[string]any{
				"tags": script.Tags,
				"name": filepath.Base(script.Path),
				"path": script.Path,
			})
			if err != nil {
				return fmt.Errorf("expression evaluation failed for script %s: %w", script.Path, err)
			}

			if enabled {
				scriptsToRun = append(scriptsToRun, script)
			}
		}
	}

	if len(scriptsToRun) == 0 {
		log.Debug().Str("type", RunnerTypeScript).Str("expr", args.Expr).Msg("no scripts matching selector found")
		return nil // nothing to run
	}

	// Create a cancellation context with signal handling
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Execute matched scripts
	for _, script := range scriptsToRun {
		// Create a cancelable context for each script
		scriptCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Print styled header for script
		fmt.Println(createStyledHeader("SCRIPT", filepath.Base(script.Path), args.TerminalWidth))
		log.Debug().
			Str("path", script.Path).
			Str("workdir", sr.cfg.ConfigDir).
			Strs("tags", script.Tags).
			Msg("Executing script")

		// Make script executable
		if err := os.Chmod(script.Path, 0o755); err != nil {
			log.Error().Err(err).Str("path", script.Path).Msg("Failed to set script permissions")
			return err
		}

		// Execute script with the configured shell
		cmd := exec.CommandContext(scriptCtx, sr.cfg.Exec.Shell, script.Path)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Dir = sr.cfg.ConfigDir // Run script in config directory

		if err := cmd.Run(); err != nil {
			log.Error().Err(err).Str("path", script.Path).Msg("Script execution failed")
			return err
		}

		// Add a newline after script execution for readability
		fmt.Println()
	}

	return nil
}

// Form implements Runner.
func (sr *ScriptRunner) Form(ctx context.Context) *huh.Group {
	sr.formsActivated = true
	sr.formsScriptMap = map[string]core.Script{}
	sr.formSelected = []string{}

	options := []huh.Option[string]{}

	for _, script := range sr.cfg.Exec.Scripts {
		displayStr := fmt.Sprintf("%s (%s)", script.Path, strings.Join(script.Tags, ", "))
		options = append(options, huh.NewOption(displayStr, script.Path))
		sr.formsScriptMap[script.Path] = script
	}

	if len(options) == 0 {
		return nil
	}

	return huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Select Scripts to Run").
			Options(options...).
			Value(&sr.formSelected),
	)
}
