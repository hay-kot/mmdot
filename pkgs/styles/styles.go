// Package styles contains the shared styles for the terminal UI components.
package styles

import (
	"github.com/charmbracelet/lipgloss"
)

type RenderFunc func(string ...string) string

const (
	Check  = "✔"
	Cross  = "✘"
	Git    = "\uf02a2" // 󰊢
	Folder = ""
	Dot    = "•"
)

const (
	ColorSuccess = "#22c55e"
	ColorError   = "#d75f6b"
	ColorSubtle  = "#a3a3a3"
)

var (
	Bold      = lipgloss.NewStyle().Bold(true).Render
	Padding   = lipgloss.NewStyle().PaddingLeft(1).Render
	Underline = lipgloss.NewStyle().Underline(true).Render

	Error   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorError)).Render
	Success = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSuccess)).PaddingLeft(1).Render
	Subtle  = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).PaddingLeft(1).Render
)

// ErrorBox creates a bordered error box with title and message
func ErrorBox(title, message string) string {
	redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorError))
	subtleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))

	lines := []string{
		redStyle.Render("╭ " + title),
		redStyle.Render("│") + " " + subtleStyle.Render(message),
		redStyle.Render("╵"),
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
