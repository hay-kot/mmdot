// Package migrations contains config version migration notes.
// Each entry describes what changed from the previous version and how to update.
package migrations

// Note describes a single config version migration.
type Note struct {
	// Version is the target version (e.g., 2 means "changes from v1 to v2").
	Version int
	// Summary is a short description of the migration for listing.
	Summary string
	// Body is the full migration guide in markdown.
	Body string
}

// Notes is the ordered list of all config migrations.
// Add new entries at the end when bumping core.ConfigVersion.
var Notes = []Note{
	{
		Version: 2,
		Summary: "Brew file generation moved to templates",
		Body: `### Migrate v1 → v2: Brew file generation moved to templates

**Removed:**
- ` + "`brew compile`" + ` subcommand
- ` + "`brews.*.outfile`" + ` config field
- Hardcoded Brewfile script generation

**Added:**
- ` + "`brewConfig`" + ` template function: resolves a named brew config (with includes) and returns the Brews struct
- ` + "`brewfile`" + ` built-in partial: renders brew tap/install/cask/mas commands

**Migration steps:**
1. Remove any ` + "`outfile`" + ` fields from your ` + "`brews:`" + ` config entries
2. Add a template entry for each brew config that had an outfile:

` + "```yaml" + `
templates:
  - name: brew-personal
    tags: [brew]
    output: ./generated/brew-personal.sh
    perm: "0755"
    template: |
      #!/bin/bash
      set -euo pipefail
      {{template "brewfile" "personal"}}
` + "```" + `

3. Set ` + "`version: 2`" + ` in your config file
4. Run ` + "`mmdot generate`" + ` instead of ` + "`mmdot brew compile`" + `
`,
	},
}
