package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

type RunnerType = string

const (
	RunnerTypeTemplate RunnerType = "template"
	RunnerTypeScript   RunnerType = "script"
)

// RunnerTypeFromStrings converts a slice of strings to a slice of RunnerType values.
// Returns an error if any string is not a valid RunnerType.
func RunnerTypeFromStrings(strs []string) ([]RunnerType, error) {
	result := make([]RunnerType, 0, len(strs))

	for i, str := range strs {
		rt := RunnerType(str)

		// Validate that the string is a valid RunnerType
		if rt != RunnerTypeTemplate && rt != RunnerTypeScript {
			return nil, fmt.Errorf("invalid runner type at index %d: %q (expected %q or %q)",
				i, str, RunnerTypeTemplate, RunnerTypeScript)
		}

		result = append(result, rt)
	}

	return result, nil
}

type ExecuteArgs struct {
	Types         []RunnerType
	TerminalWidth int    // Width of the Terminal
	Expr          string // Evaluation Expression
}

type Runner interface {
	// Form returns a group reference that is mounted to a huh.Form for users to select
	// what actions they want to perform. The [Action] implementer should store the
	// internal state of the selected values for execution later.
	Form(ctx context.Context) *huh.Group

	// Execute the configured [Actions]
	Execute(ctx context.Context, args ExecuteArgs) error
}

// compileExpr compiles an expression string once for reuse
func compileExpr(code string) (*vm.Program, error) {
	if code == "" {
		code = "true" // default: match everything
	}

	return expr.Compile(code, expr.AsBool())
}

// evalCompiledExpr evaluates a pre-compiled expression with given context
func evalCompiledExpr(program *vm.Program, env map[string]any) (bool, error) {
	output, err := expr.Run(program, env)
	if err != nil {
		return false, err
	}

	// expr.AsBool() ensures output is always bool
	result, ok := output.(bool)
	if !ok {
		return false, fmt.Errorf("expression did not evaluate to boolean, got %T", output)
	}

	return result, nil
}

var (
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Bold(true)
	bracketStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	nameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#c0caf5"))
	dividerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
)

// createStyledHeader creates a styled header for templates and scripts
func createStyledHeader(label, name string, terminalWidth int) string {
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
