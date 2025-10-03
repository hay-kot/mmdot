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
	"github.com/charmbracelet/lipgloss"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/internal/generator"
	"github.com/hay-kot/mmdot/pkgs/printer"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

type RunCmd struct {
	coreFlags *core.Flags
	flags     struct {
		Tags  []string
		Types []string
		List  bool
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
		Usage:     "Execute scripts and generate templates from the mmdot.yaml configuration",
		ArgsUsage: "[group]",
		Description: `Execute scripts and generate templates defined in your mmdot.yaml configuration file.
 Items can be run by specifying a group (which resolves to tags), filtering by tags and type,
 or through interactive selection.

 Examples:
	 mmdot run personal                  # Run all templates and scripts from 'personal' group
	 mmdot run --tags work               # Run all items tagged with 'work'
	 mmdot run --type template           # Generate all templates
	 mmdot run --type script             # Run all scripts
	 mmdot run personal --type template  # Generate templates from 'personal' group
	 mmdot run --list personal           # List items in 'personal' without executing
	 mmdot run                           # Interactive selection`,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:        "tags",
				Aliases:     []string{"t"},
				Usage:       "filter scripts and templates by tags (can specify multiple)",
				Destination: &sc.flags.Tags,
			},
			&cli.StringSliceFlag{
				Name:        "type",
				Usage:       "filter by type: 'template' or 'script' (default: both)",
				Destination: &sc.flags.Types,
			},
			&cli.BoolFlag{
				Name:        "list",
				Aliases:     []string{"ls", "l"},
				Usage:       "list matching items without executing them",
				Destination: &sc.flags.List,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "group",
				UsageText:   "group name to be applied to arguments",
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

	// Determine which types to include
	includeTemplates := sc.shouldIncludeType("template")
	includeScripts := sc.shouldIncludeType("script")

	// Gather scripts and templates based on selection mode
	var matchedScripts []core.Script
	var matchedTemplates []core.Template
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

		// Filter scripts and templates by tags
		if includeScripts {
			matchedScripts = filterScriptsByTags(cfg.Exec.Scripts, tagsToFilter)
		}
		if includeTemplates {
			matchedTemplates = filterTemplatesByTags(cfg.Templates, tagsToFilter)
		}

	case len(sc.flags.Tags) > 0:
		// Filter by tags from flags
		if includeScripts {
			matchedScripts = filterScriptsByTags(cfg.Exec.Scripts, sc.flags.Tags)
		}
		if includeTemplates {
			matchedTemplates = filterTemplatesByTags(cfg.Templates, sc.flags.Tags)
		}

	default:
		// Interactive selection mode with form for templates and scripts
		var formGroups []*huh.Group

		selectedTemplates := []string{}
		selectedScripts := []string{}

		if includeTemplates {
			templateOptions := []huh.Option[string]{}
			templateMap := make(map[string]core.Template)

			for _, t := range cfg.Templates {
				displayStr := fmt.Sprintf("%s (%s)", t.Name, strings.Join(t.Tags, ", "))
				templateOptions = append(templateOptions, huh.NewOption(displayStr, t.Name))
				templateMap[t.Name] = t
			}

			if len(templateOptions) > 0 {
				formGroups = append(formGroups, huh.NewGroup(
					huh.NewMultiSelect[string]().
						Title("Select Templates to Generate").
						Options(templateOptions...).
						Value(&selectedTemplates),
				))
			}

			// Store template map for later use
			defer func() {
				for _, selectedName := range selectedTemplates {
					if template, ok := templateMap[selectedName]; ok {
						matchedTemplates = append(matchedTemplates, template)
					}
				}
			}()
		}

		if includeScripts {
			scriptOptions := []huh.Option[string]{}
			scriptMap := make(map[string]core.Script)

			for _, s := range cfg.Exec.Scripts {
				displayStr := fmt.Sprintf("%s (%s)", s.Path, strings.Join(s.Tags, ", "))
				scriptOptions = append(scriptOptions, huh.NewOption(displayStr, s.Path))
				scriptMap[s.Path] = s
			}

			if len(scriptOptions) > 0 {
				formGroups = append(formGroups, huh.NewGroup(
					huh.NewMultiSelect[string]().
						Title("Select Scripts to Run").
						Options(scriptOptions...).
						Value(&selectedScripts),
				))
			}

			// Store script map for later use
			defer func() {
				for _, selectedPath := range selectedScripts {
					if script, ok := scriptMap[selectedPath]; ok {
						matchedScripts = append(matchedScripts, script)
					}
				}
			}()
		}

		if len(formGroups) > 0 {
			form := huh.NewForm(formGroups...)
			if err := form.Run(); err != nil {
				return err
			}
		}
	}

	if len(matchedScripts) == 0 && len(matchedTemplates) == 0 {
		fmt.Println("No scripts or templates matched the specified criteria")
		return nil
	}

	// If list flag is set, just list the templates and scripts without executing
	if sc.flags.List {
		p := printer.New(os.Stdout)
		p.LineBreak()

		if len(matchedTemplates) > 0 {
			var items []string
			for _, template := range matchedTemplates {
				items = append(items, fmt.Sprintf("%s (tags: %s)", template.Name, strings.Join(template.Tags, ", ")))
			}
			p.List("Templates to generate:", items)
			p.LineBreak()
		}

		if len(matchedScripts) > 0 {
			var items []string
			for _, script := range matchedScripts {
				items = append(items, fmt.Sprintf("%s (tags: %s)", script.Path, strings.Join(script.Tags, ", ")))
			}
			p.List("Scripts to run:", items)
		}
		return nil
	}

	// Generate templates FIRST before running scripts
	if len(matchedTemplates) > 0 {
		engine := generator.NewEngine(&cfg)

		for _, tmpl := range matchedTemplates {
			// Print styled header for template
			fmt.Println(createStyledHeader("TEMPLATE", tmpl.Name, terminalWidth))

			if err := engine.RenderTemplate(ctx, tmpl); err != nil {
				return fmt.Errorf("failed to generate template %s: %w", tmpl.Name, err)
			}

			// Print template section
			pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7"))
			successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
			templateContentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9aa5ce"))

			// Print Output Path and Status
			fmt.Printf("Status       %s\n", successStyle.Render("Rendered"))
			fmt.Printf("Output Path  %s\n", pathStyle.Render(tmpl.Output))
			fmt.Println()

			// Print Template Body label and content
			fmt.Println("Template Body:")
			templateLines := strings.SplitSeq(tmpl.Template, "\n")
			for line := range templateLines {
				fmt.Println(templateContentStyle.Render("  " + line))
			}

			fmt.Println() // Add blank line after template generation
		}
	}

	// Create a cancellation context with signal handling
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Execute matched scripts
	for _, script := range matchedScripts {
		// Create a cancelable context for each script
		scriptCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Print styled header for script
		fmt.Println(createStyledHeader("SCRIPT", filepath.Base(script.Path), terminalWidth))
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

// shouldIncludeType determines if the given type should be included based on --type flags
func (sc *RunCmd) shouldIncludeType(typeName string) bool {
	// If no types specified, include everything
	if len(sc.flags.Types) == 0 {
		return true
	}

	// Check if the type is in the list
	return slices.Contains(sc.flags.Types, typeName)
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
			found := slices.Contains(script.Tags, requiredTag)
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

// filterTemplatesByTags returns templates that match all specified tags
func filterTemplatesByTags(templates []core.Template, tags []string) []core.Template {
	if len(tags) == 0 {
		return templates
	}

	var filtered []core.Template

	for _, template := range templates {
		// Check if template has all the required tags
		hasAllTags := true
		for _, requiredTag := range tags {
			found := slices.Contains(template.Tags, requiredTag)
			if !found {
				hasAllTags = false
				break
			}
		}

		if hasAllTags {
			filtered = append(filtered, template)
		}
	}

	return filtered
}

// createStyledHeader creates a styled header for templates and scripts
func createStyledHeader(label, name string, terminalWidth int) string {
	// Create styled label with brackets and color
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7aa2f7")).
		Bold(true)

	bracketStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#565f89"))

	nameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#c0caf5"))

	dividerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#565f89"))

	// Build the header parts
	leftPart := fmt.Sprintf("%s %s%s%s %s ",
		dividerStyle.Render("--"),
		bracketStyle.Render("["),
		labelStyle.Render(label),
		bracketStyle.Render("]"),
		nameStyle.Render(name),
	)

	// Calculate visible length (excluding ANSI codes)
	// Approximate: "-- [LABEL] name "
	visibleLength := 4 + len(label) + len(name) + 4 // "-- " + "[" + label + "]" + " " + name + " "

	// Fill remaining space with dashes
	remainingSpace := max(terminalWidth-visibleLength, 0)

	divider := dividerStyle.Render(strings.Repeat("-", remainingSpace))

	return leftPart + divider
}
