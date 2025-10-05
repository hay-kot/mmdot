#!/bin/bash
set -e

# Check if on main branch
BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$BRANCH" != "main" ]; then
  echo "Error: Must be on main branch to release (currently on $BRANCH)"
  exit 1
fi

# Run PR checks
echo "Running PR checks..."
task pr
if [ $? -ne 0 ]; then
  echo "Error: PR checks failed"
  exit 1
fi

# Choose version bump type
BUMP_TYPE=$(echo -e "patch\nminor\nmajor" | fzf --prompt="Select version bump type: ")
if [ -z "$BUMP_TYPE" ]; then
  echo "Error: No version type selected"
  exit 1
fi

# Get next version using svu
NEXT_VERSION=$(svu $BUMP_TYPE)
if [ -z "$NEXT_VERSION" ]; then
  echo "Error: Failed to determine next version"
  exit 1
fi

echo "Next version: $NEXT_VERSION"

# Update version in main.go
sed -i '' "s/version = \".*\"/version = \"$NEXT_VERSION\"/" main.go

# Commit version bump
git add main.go
git commit -m "chore: bump version to $NEXT_VERSION"

# Create git tag
git tag "$NEXT_VERSION"

# Push commit and tag
git push origin main
git push origin "$NEXT_VERSION"

# Create GitHub release with automatic notes
gh release create "$NEXT_VERSION" --generate-notes

# Update version to -develop
sed -i '' "s/version = \".*\"/version = \"$NEXT_VERSION-develop\"/" main.go

# Commit develop version
git add main.go
git commit -m "chore: set version to $NEXT_VERSION-develop"
git push origin main

echo "Release $NEXT_VERSION completed successfully!"
