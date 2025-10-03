package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hay-kot/mmdot/internal/core"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

type HookCmd struct {
	coreFlags *core.Flags
}

func NewHookCmd(coreFlags *core.Flags) *HookCmd {
	return &HookCmd{coreFlags: coreFlags}
}

func (hc *HookCmd) Register(app *cli.Command) *cli.Command {
	cmds := []*cli.Command{
		{
			Name:  "hook",
			Usage: "manage git hooks for mmdot",
			Commands: []*cli.Command{
				{
					Name:  "install",
					Usage: "install git pre-commit hook to check for unencrypted vault files",
					Description: `Installs a pre-commit hook that prevents commits containing unencrypted vault files.

The hook will call 'mmdot encrypt --dry-run' before each commit to verify that all vault files
are properly encrypted with .age extension.

If a pre-commit hook already exists, the mmdot check will be appended to it.`,
					Action: hc.install,
				},
				{
					Name:  "uninstall",
					Usage: "remove the mmdot pre-commit hook",
					Description: `Removes the mmdot pre-commit hook from .git/hooks/

This will only remove the mmdot section from hooks that were created/modified by 'mmdot hook install'.`,
					Action: hc.uninstall,
				},
			},
		},
	}

	app.Commands = append(app.Commands, cmds...)
	return app
}

func (hc *HookCmd) install(ctx context.Context, cmd *cli.Command) error {
	// Find .git directory
	gitDir, err := findGitDir()
	if err != nil {
		return fmt.Errorf("failed to find .git directory: %w", err)
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	hookPath := filepath.Join(hooksDir, "pre-commit")

	// Create hooks directory if it doesn't exist
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	// Get the current mmdot binary path
	mmdotPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get mmdot executable path: %w", err)
	}

	// Get config path relative to git root if possible
	configPath := hc.coreFlags.ConfigFilePath
	gitRoot := filepath.Dir(gitDir)
	if relPath, err := filepath.Rel(gitRoot, configPath); err == nil && !strings.HasPrefix(relPath, "..") {
		configPath = relPath
	}

	// Create the mmdot hook section
	mmdotHook := fmt.Sprintf(`
# mmdot pre-commit hook - check vault files are encrypted
%s --config="%s" encrypt --dry-run || exit 1
`, mmdotPath, configPath)

	var hookContent string

	// Check if hook already exists
	if existingContent, err := os.ReadFile(hookPath); err == nil {
		// Hook exists, check if our section is already there
		if strings.Contains(string(existingContent), "mmdot pre-commit hook") {
			log.Info().Str("path", hookPath).Msg("mmdot pre-commit hook already installed")
			return nil
		}

		// Append to existing hook
		hookContent = string(existingContent) + mmdotHook
		log.Info().Str("path", hookPath).Msg("Appending mmdot check to existing pre-commit hook")
	} else {
		// Create new hook with shebang
		hookContent = "#!/bin/sh\n" + mmdotHook
		log.Info().Str("path", hookPath).Msg("Creating new pre-commit hook with mmdot check")
	}

	// Write the hook file
	if err := os.WriteFile(hookPath, []byte(hookContent), 0755); err != nil {
		return fmt.Errorf("failed to write pre-commit hook: %w", err)
	}

	log.Info().Msg("Installed pre-commit hook successfully")
	return nil
}

func (hc *HookCmd) uninstall(ctx context.Context, cmd *cli.Command) error {
	// Find .git directory
	gitDir, err := findGitDir()
	if err != nil {
		return fmt.Errorf("failed to find .git directory: %w", err)
	}

	hookPath := filepath.Join(gitDir, "hooks", "pre-commit")

	// Check if hook exists
	content, err := os.ReadFile(hookPath)
	if os.IsNotExist(err) {
		log.Info().Msg("No pre-commit hook found")
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read pre-commit hook: %w", err)
	}

	// Check if our section exists
	if !strings.Contains(string(content), "mmdot pre-commit hook") {
		log.Info().Msg("mmdot hook not found in pre-commit")
		return nil
	}

	// Remove our section
	lines := strings.Split(string(content), "\n")
	var newLines []string
	inMmdotSection := false

	for _, line := range lines {
		if strings.Contains(line, "mmdot pre-commit hook") {
			inMmdotSection = true
			continue
		}
		if inMmdotSection && (strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#")) {
			// Skip comment lines and empty lines in mmdot section
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
		}
		if inMmdotSection && strings.Contains(line, "encrypt --dry-run") {
			inMmdotSection = false
			continue
		}

		newLines = append(newLines, line)
	}

	newContent := strings.Join(newLines, "\n")

	// If the file is now empty or only has shebang, remove it entirely
	trimmed := strings.TrimSpace(newContent)
	if trimmed == "" || trimmed == "#!/bin/sh" {
		if err := os.Remove(hookPath); err != nil {
			return fmt.Errorf("failed to remove pre-commit hook: %w", err)
		}
		log.Info().Str("path", hookPath).Msg("Removed empty pre-commit hook")
		return nil
	}

	// Write back the modified hook
	if err := os.WriteFile(hookPath, []byte(newContent), 0755); err != nil {
		return fmt.Errorf("failed to write pre-commit hook: %w", err)
	}

	log.Info().Str("path", hookPath).Msg("Removed mmdot section from pre-commit hook")
	return nil
}

// findGitDir finds the .git directory by walking up from current directory
func findGitDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return gitDir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a git repository")
		}
		dir = parent
	}
}
