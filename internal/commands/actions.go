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
	TerminalWidth int               // Width of the Terminal
	Expr          string            // Evaluation Expression
	Macros        map[string]string // Macro definitions for expression expansion
	List          bool              // List matching items without executing
}

type Runner interface {
	// Form returns a group reference that is mounted to a huh.Form for users to select
	// what actions they want to perform. The [Action] implementer should store the
	// internal state of the selected values for execution later.
	Field(ctx context.Context) huh.Field

	// Execute the configured [Actions]
	Execute(ctx context.Context, args ExecuteArgs) error
}

// expandMacros replaces @macroname references with their values from the macros map
func expandMacros(code string, macros map[string]string) (string, error) {
	if code == "" {
		return code, nil
	}

	// Check for macro references in the code
	if strings.Contains(code, "@") {
		// Find all @macroname references and validate they exist
		words := strings.FieldsFunc(code, func(r rune) bool {
			return r == ' ' || r == '(' || r == ')' || r == '&' || r == '|' || r == '!' || r == '=' || r == '"' || r == '\''
		})

		for _, word := range words {
			if strings.HasPrefix(word, "@") {
				macroName := strings.TrimPrefix(word, "@")
				if _, exists := macros[macroName]; !exists {
					return "", fmt.Errorf("undefined macro: @%s", macroName)
				}
			}
		}
	}

	result := code
	for key, value := range macros {
		// Replace @macroname with the macro value
		// Use strings.ReplaceAll to replace all occurrences
		placeholder := "@" + key
		result = strings.ReplaceAll(result, placeholder, "("+value+")")
	}

	return result, nil
}

// compileExpr compiles an expression string once for reuse
func compileExpr(code string, macros map[string]string) (*vm.Program, error) {
	if code == "" {
		code = "true" // default: match everything
	}

	// Expand macros before compiling
	expanded, err := expandMacros(code, macros)
	if err != nil {
		return nil, fmt.Errorf("failed to expand macros: %w", err)
	}

	return expr.Compile(expanded, expr.AsBool())
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

// ListItem represents an item to be displayed in a list
type ListItem struct {
	Name string
	Tags []string
}

// printList prints a formatted list with aligned tags
func printList(title string, items []ListItem) {
	var (
		titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Bold(true).Underline(true)
		nameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#c0caf5"))
		tagStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).Italic(true)
	)

	fmt.Println(" " + titleStyle.Render(title))

	// Find longest name for alignment
	maxNameLen := 0
	for _, item := range items {
		if len(item.Name) > maxNameLen {
			maxNameLen = len(item.Name)
		}
	}

	for _, item := range items {
		tags := ""
		if len(item.Tags) > 0 {
			tags = " " + tagStyle.Render("("+strings.Join(item.Tags, ", ")+")")
		}
		padding := strings.Repeat(" ", maxNameLen-len(item.Name))
		fmt.Printf("  - %s%s%s\n", nameStyle.Render(item.Name), padding, tags)
	}
	fmt.Println()
}
