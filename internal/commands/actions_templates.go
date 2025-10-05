package commands

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/internal/generator"
	"github.com/rs/zerolog/log"
)

var _ Runner = &TemplateRunner{}

type TemplateRunner struct {
	cfg    *core.ConfigFile
	engine generator.Engine

	formsActivated   bool
	formsTemplateMap map[string]core.Template
	formSelected     []string
}

func NewTemplateRunner(cfg *core.ConfigFile) *TemplateRunner {
	return &TemplateRunner{
		cfg:              cfg,
		engine:           *generator.NewEngine(cfg),
		formsActivated:   false,
		formsTemplateMap: map[string]core.Template{},
		formSelected:     []string{},
	}
}

// Execute implements Runner.
func (tr *TemplateRunner) Execute(ctx context.Context, args ExecuteArgs) error {
	if !slices.Contains(args.Types, RunnerTypeTemplate) {
		log.Debug().Str("type", RunnerTypeTemplate).Msg("type disabled")
		return nil // nothing to run
	}

	templatesToRun := []core.Template{}

	switch {
	case tr.formsActivated: // Assume for has run and we have user interactions to base selection on
		for _, selected := range tr.formSelected {
			templatesToRun = append(templatesToRun, tr.formsTemplateMap[selected])
		}
	default:
		// Compile expression once before loop
		program, err := compileExpr(args.Expr, args.Macros)
		if err != nil {
			return fmt.Errorf("invalid expression: %w", err)
		}

		for _, tmpl := range tr.cfg.Templates {
			enabled, err := evalCompiledExpr(program, map[string]any{
				"tags": tmpl.Tags,
				"name": tmpl.Name,
			})
			if err != nil {
				return fmt.Errorf("expression evaluation failed for template %s: %w", tmpl.Name, err)
			}

			if enabled {
				templatesToRun = append(templatesToRun, tmpl)
			}
		}
	}

	if len(templatesToRun) == 0 {
		log.Debug().Str("type", RunnerTypeTemplate).Str("expr", args.Expr).Msg("no templates matching selector found")
		return nil // nothing to run
	}

	var (
		pathStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7"))
		successStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
		templateContentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9aa5ce"))
	)

	for _, tmpl := range templatesToRun {
		// Print styled header for template
		fmt.Println(createStyledHeader("TEMPLATE", tmpl.Name, args.TerminalWidth))

		if err := tr.engine.RenderTemplate(ctx, tmpl); err != nil {
			return fmt.Errorf("failed to generate template %s: %w", tmpl.Name, err)
		}

		log.Debug().
			Str("template", tmpl.Name).
			Str("output", tmpl.Output).
			Strs("tags", tmpl.Tags).
			Msg("rendered template")

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

	return nil
}

// Form implements Runner.
func (tr *TemplateRunner) Form(ctx context.Context) *huh.Group {
	tr.formsActivated = true
	tr.formsTemplateMap = map[string]core.Template{}
	tr.formSelected = []string{}

	options := []huh.Option[string]{}

	for _, tmpl := range tr.cfg.Templates {
		displayStr := fmt.Sprintf("%s (%s)", tmpl.Name, strings.Join(tmpl.Tags, ", "))
		options = append(options, huh.NewOption(displayStr, tmpl.Name))
		tr.formsTemplateMap[tmpl.Name] = tmpl
	}

	if len(options) == 0 {
		return nil
	}

	return huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Select Templates to Generate").
			Options(options...).
			Value(&tr.formSelected),
	)
}
