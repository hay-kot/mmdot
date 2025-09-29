package generator

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type TemplateError struct {
	File    string
	Line    int
	Column  int
	Message string
	Context []string
}

func NewTemplateError(file string, err error) *TemplateError {
	te := &TemplateError{
		File:    file,
		Message: err.Error(),
	}

	te.parseError(err.Error())
	te.loadContext()
	te.cleanMessage()

	return te
}

func (te *TemplateError) parseError(errStr string) {
	// Parse Go template error format: template: name:line:col: error message
	// Example: template: test.tmpl:5:14: executing "test.tmpl" at <.unknownVar>: can't evaluate field unknownVar
	re := regexp.MustCompile(`template: [^:]+:(\d+):(\d+): (.+)`)
	matches := re.FindStringSubmatch(errStr)
	if len(matches) > 3 {
		if line, err := strconv.Atoi(matches[1]); err == nil {
			te.Line = line
		}
		if col, err := strconv.Atoi(matches[2]); err == nil {
			te.Column = col
		}
		te.Message = matches[3]
	}

	// Also try parsing simpler format: template: name:line: error
	if te.Line == 0 {
		re = regexp.MustCompile(`template: [^:]+:(\d+): (.+)`)
		matches = re.FindStringSubmatch(errStr)
		if len(matches) > 2 {
			if line, err := strconv.Atoi(matches[1]); err == nil {
				te.Line = line
			}
			te.Message = matches[2]
		}
	}
}

func (te *TemplateError) loadContext() {
	if te.Line == 0 {
		return
	}

	file, err := os.Open(te.File)
	if err != nil {
		return
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	lineNum := 1
	var lines []string

	for scanner.Scan() {
		line := scanner.Text()
		if lineNum >= te.Line-2 && lineNum <= te.Line+2 {
			lines = append(lines, line)
		}
		if lineNum > te.Line+2 {
			break
		}
		lineNum++
	}

	te.Context = lines
}

func (te *TemplateError) cleanMessage() {
	// Clean up common template error messages
	replacements := map[string]string{
		"can't evaluate field":     "unknown field",
		"map has no entry for key": "missing key",
		"executing":                "error in",
		"at <":                     "accessing variable <",
	}

	for old, new := range replacements {
		te.Message = strings.ReplaceAll(te.Message, old, new)
	}

	// Remove redundant template name references
	baseName := filepath.Base(te.File)
	te.Message = strings.ReplaceAll(te.Message, fmt.Sprintf(`"%s" `, baseName), "")
	te.Message = strings.ReplaceAll(te.Message, fmt.Sprintf(`executing "%s" `, baseName), "")
}

func (te *TemplateError) Error() string {
	return te.format()
}

func (te *TemplateError) format() string {
	if te.Line == 0 {
		return fmt.Sprintf("Template error in %s: %s", te.File, te.Message)
	}

	// Styles using more muted, theme-friendly colors
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)     // Muted red
	fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Underline(true) // Blue
	lineNumStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))              // Dark gray
	errorLineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // Dark red
	contextStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))              // Light gray
	pointerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)   // Yellow

	var sb strings.Builder

	// Error header
	sb.WriteString(errorStyle.Render("Template Error") + "\n\n")

	// File location
	location := fmt.Sprintf("%s:%d", te.File, te.Line)
	if te.Column > 0 {
		location += fmt.Sprintf(":%d", te.Column)
	}
	sb.WriteString(fileStyle.Render(location) + "\n\n")

	// Context with error line highlighted
	if len(te.Context) > 0 {
		startLine := max(te.Line-2, 1)

		for i, line := range te.Context {
			currentLine := startLine + i
			lineNumStr := fmt.Sprintf("%4d │ ", currentLine)

			if currentLine == te.Line {
				// Error line
				sb.WriteString(errorLineStyle.Render(lineNumStr))
				sb.WriteString(errorLineStyle.Render(line) + "\n")

				// Add pointer if we have column info
				if te.Column > 0 && te.Column <= len(line) {
					spaces := strings.Repeat(" ", 6+te.Column-1)
					sb.WriteString(spaces + pointerStyle.Render("^") + "\n")
				}
			} else {
				// Context line
				sb.WriteString(lineNumStyle.Render(lineNumStr))
				sb.WriteString(contextStyle.Render(line) + "\n")
			}
		}
		sb.WriteString("\n")
	}

	// Error message
	sb.WriteString(errorStyle.Render("Error: ") + te.Message + "\n")

	return sb.String()
}

