#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e
# Exit if any command in a pipeline fails (not just the last one)
set -o pipefail
# Treat unset variables as an error when substituting
set -u
# Adding Homebrew Taps
brew untap remove:taps

# Installing Homebrew Packages
brew uninstall remove:brew

# Installing Homebrew Casks
brew uninstall --cask remove:cask

