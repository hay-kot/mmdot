package commands

import (
	"strings"
	"testing"

	"github.com/hay-kot/mmdot/internal/core"
)

func TestWriteMigrationNotes_FromV1(t *testing.T) {
	var out strings.Builder
	writeMigrationNotes(&out, 1, core.ConfigVersion)

	result := out.String()
	if result == "" {
		t.Fatal("expected migration notes, got empty string")
	}

	for _, want := range []string{
		"v1 → v2",
		"brew compile",
		"brewfile",
		"brewConfig",
		"outfile",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("migration notes missing %q", want)
		}
	}
}

func TestWriteMigrationNotes_CurrentVersion(t *testing.T) {
	var out strings.Builder
	writeMigrationNotes(&out, core.ConfigVersion, core.ConfigVersion)

	if out.String() != "" {
		t.Errorf("expected no migration notes for current version, got: %s", out.String())
	}
}

func TestEmbeddedConfigSchema(t *testing.T) {
	content := mustReadEmbed("llmtext/config_schema.txt")

	for _, want := range []string{
		"version:",
		"templates:",
		"brews:",
		"variables:",
		"age:",
		"Variable precedence",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("schema reference missing %q", want)
		}
	}
}

func TestEmbeddedTemplateDocs(t *testing.T) {
	content := mustReadEmbed("llmtext/template_docs.txt")

	for _, want := range []string{
		"brewConfig",
		"brewfile",
		"Built-in Partials",
		"Template Functions",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("template docs missing %q", want)
		}
	}
}
