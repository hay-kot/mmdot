package commands

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/internal/migrations"
	"github.com/urfave/cli/v3"
)

//go:embed llmtext/*.txt
var llmtextFS embed.FS

type LLMTextCmd struct {
	flags *core.Flags
}

func NewLLMTextCmd(flags *core.Flags) *LLMTextCmd {
	return &LLMTextCmd{flags: flags}
}

func (lc *LLMTextCmd) Register(app *cli.Command) *cli.Command {
	cmd := &cli.Command{
		Name:  "llmtext",
		Usage: "Output LLM-consumable documentation",
		Commands: []*cli.Command{
			{
				Name:  "config",
				Usage: "Output config schema, template functions, and migration notes",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "version",
						Usage: "override config version (default: read from config file)",
					},
				},
				Action: lc.config,
			},
		},
	}

	app.Commands = append(app.Commands, cmd)
	return app
}

func (lc *LLMTextCmd) config(ctx context.Context, c *cli.Command) error {
	userVersion := int(c.Int("version"))

	if userVersion == 0 {
		cfg, err := core.SetupEnv(lc.flags.ConfigFilePath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		userVersion = cfg.Version
	}

	var out strings.Builder

	fmt.Fprintf(&out, "# mmdot config (current: v%d)\n\n", core.ConfigVersion)

	// Migration notes
	if userVersion < core.ConfigVersion {
		fmt.Fprintf(&out, "## Your config: v%d — migrations needed\n\n", userVersion)
		writeMigrationNotes(&out, userVersion, core.ConfigVersion)
	} else {
		fmt.Fprintf(&out, "## Your config: v%d — up to date\n\n", userVersion)
	}

	// Schema reference
	out.WriteString(mustReadEmbed("llmtext/config_schema.txt"))
	out.WriteString("\n")

	// Template functions and partials
	out.WriteString(mustReadEmbed("llmtext/template_docs.txt"))
	out.WriteString("\n")

	// Migration history
	writeMigrationHistory(&out)

	fmt.Print(out.String())
	return nil
}

func mustReadEmbed(path string) string {
	data, err := llmtextFS.ReadFile(path)
	if err != nil {
		panic("failed to read embedded file " + path + ": " + err.Error())
	}
	return string(data)
}

func writeMigrationNotes(out *strings.Builder, from, to int) {
	for _, note := range migrations.Notes {
		if note.Version > from && note.Version <= to {
			out.WriteString(note.Body)
			out.WriteString("\n")
		}
	}
}

func writeMigrationHistory(out *strings.Builder) {
	out.WriteString("## Migration History\n\n")
	for _, note := range migrations.Notes {
		fmt.Fprintf(out, "- v%d → v%d: %s\n", note.Version-1, note.Version, note.Summary)
	}
	out.WriteString("\n")
}
