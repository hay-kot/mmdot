#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e
# Exit if any command in a pipeline fails (not just the last one)
set -o pipefail
# Treat unset variables as an error when substituting
set -u
# Adding Homebrew Taps
brew tap all:taps
brew tap personal:taps

# Installing Homebrew Packages
brew install \
  all:brews \
  personal:brews

# Installing Homebrew Casks
brew install --cask \
  all:casks \
  personal:casks

