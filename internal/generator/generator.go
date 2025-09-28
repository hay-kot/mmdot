package generator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/charmbracelet/lipgloss"
)

type Generator struct {
	config       *Config
	successCount int
}

func New(config *Config) *Generator {
	return &Generator{
		config: config,
	}
}

func (g *Generator) Generate() error {
	g.successCount = 0

	for _, job := range g.config.Jobs {
		if err := g.processJob(&job); err != nil {
			// Return template errors directly without wrapping
			if _, ok := err.(*TemplateError); ok {
				return err
			}
			return fmt.Errorf("failed to process job %s: %w", job.Template, err)
		}
		g.successCount++
	}

	g.printSummary()
	return nil
}

func (g *Generator) processJob(job *Job) error {
	tmplContent, err := os.ReadFile(job.Template)
	if err != nil {
		return fmt.Errorf("failed to read template file %s: %w", job.Template, err)
	}

	tmpl := template.New(filepath.Base(job.Template))

	// Apply strict mode if enabled
	if g.config.StrictMode {
		tmpl = tmpl.Option("missingkey=error")
	}

	tmpl, err = tmpl.Parse(string(tmplContent))
	if err != nil {
		return NewTemplateError(job.Template, err)
	}

	vars := job.MergeVars(g.config.Vars)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return NewTemplateError(job.Template, err)
	}

	if err := os.MkdirAll(filepath.Dir(job.Output), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := os.WriteFile(job.Output, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	g.printJobSuccess(job)
	return nil
}

func (g *Generator) printJobSuccess(job *Job) {
	// Minimal styling - just success indicator and muted arrow
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // Green
	arrowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))   // Gray

	// Shorten paths for display
	templatePath := g.shortenPath(job.Template)
	outputPath := g.shortenPath(job.Output)

	fmt.Printf("%s %s %s %s\n",
		successStyle.Render("✓"),
		templatePath,
		arrowStyle.Render("→"),
		outputPath,
	)
}

func (g *Generator) printSummary() {
	if g.successCount == 0 {
		return
	}

	var verb string
	if g.successCount == 1 {
		verb = "template"
	} else {
		verb = "templates"
	}

	fmt.Printf("\n%d %s generated successfully\n",
		g.successCount,
		verb,
	)
}

func (g *Generator) shortenPath(path string) string {
	// Get relative path from current working directory if possible
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil && !filepath.IsAbs(rel) {
			return rel
		}
	}

	// Otherwise, just return the base name and parent directory
	dir := filepath.Base(filepath.Dir(path))
	file := filepath.Base(path)
	return filepath.Join(dir, file)
}