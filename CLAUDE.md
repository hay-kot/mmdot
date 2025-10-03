# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

mmdot is a personal dotfiles management utility written in Go. It provides functionality for:
- Template generation with variable substitution (including encrypted variables)
- Homebrew package management and diffing
- Script execution with tag-based filtering
- Age encryption/decryption for secrets management

## Development Commands

### Running the Application
```bash
# Run with default config (mmdot.yml in current directory)
task run

# Run with custom config and CLI args
task run -- --config="/path/to/config.yml" <command>

# Direct execution
go run *.go --config="./mmdot.yml" <command>
```

### Testing
```bash
# Run all tests
task test

# Run tests in watch mode
task test:watch

# Run tests with coverage
task coverage
```

### Code Quality
```bash
# Run linter
task lint

# Tidy dependencies
task tidy

# Run all PR checks (tidy, lint, test)
task pr
```

### Building
```bash
# Build with goreleaser
task build
```

## Architecture

### Core Components

**main.go**: CLI entry point using urfave/cli/v3. Registers subcommands and handles global flags (log-level, config path). Uses a deferred writer pattern for output buffering.

**internal/core/config.go**: Configuration parsing from YAML. The `ConfigFile` struct defines the main config structure with sections for templates, variables, age encryption, brew packages, and exec scripts. Changes working directory to config file location on load.

**internal/generator/engine.go**: Template rendering engine. Loads variables from multiple sources (inline vars, var files, encrypted vault files), merges them with template-specific vars, and renders Go templates to output files with configurable permissions.

**pkgs/fcrypt/**: Age encryption wrapper. Handles file encryption/decryption using the age library. Provides both in-place operations and reader/writer interfaces.

**pkgs/printer/**: Custom output formatting with deferred writing. Uses charmbracelet/lipgloss for styling. Context-based printer pattern allows buffered output that flushes on program exit.

### Command Structure

Commands are registered via the `subcommand` interface pattern:
- **generate**: Renders templates from config, supports encrypted variable files
- **brew**: Manages Homebrew packages (diff against installed, compile to Brewfile)
- **run**: Executes shell scripts with tag-based filtering and interactive selection
- **encrypt/decrypt**: Manages age-encrypted secrets files

### Configuration Flow

1. Config file is read and parsed into `ConfigFile` struct
2. Working directory changes to config file directory (all paths are relative to config)
3. For template generation:
   - Global vars loaded from `variables.vars`
   - File vars loaded from `variables.var_files` (with vault decryption if needed)
   - Template-specific vars merged in (later sources override earlier)
   - Template rendered and written with specified permissions

### Age Encryption

- Uses age public key encryption (filippo.io/age)
- Identity file (private key) stored at `age.identity_file`
- Recipients (public keys) in `age.recipients` array
- Variable files marked with `?vault=true` query parameter are automatically decrypted
- Encrypted files use `.age` extension

### Brew Management

- ConfigMap structure allows nested brew configurations with includes
- `brew diff` compares config against `brew list` output
- `brew compile` generates Brewfile-format output files
- Supports taps, brews, casks, and mas packages

## Configuration File Structure

The `mmdot.yml` configuration uses these main sections:
- `groups`: Define named groups mapping to tag lists
- `variables`: Global vars and var_files (with vault support)
- `templates`: Template definitions with inline content, output path, and permissions
- `exec`: Shell scripts with paths and tags
- `brew`: Homebrew package configurations (nested with includes)
- `age`: Encryption configuration (recipients and identity file)

## Environment Variables

All flags can be set via environment variables with `MMDOT_` prefix:
- `MMDOT_LOG_LEVEL`: Set logging level (debug, info, warn, error)
- `MMDOT_CONFIG_PATH`: Path to config file

## Code Conventions

- Use zerolog for structured logging
- Permissions specified as octal strings (e.g., "0600")
- All paths in config are relative to config file directory
- Template variables merged with precedence: global < file < template-specific
- Commands support both CLI flags and interactive prompts (using charmbracelet/huh)
