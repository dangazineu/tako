#!/bin/sh
#
# Installs the git hooks from the scripts/ directory.

# This script creates a symbolic link from the project's .git/hooks/ directory
# to the scripts/ directory. This allows the hooks to be version-controlled.

HOOKS_DIR=$(git rev-parse --git-dir)/hooks
SCRIPTS_DIR=$(git rev-parse --show-toplevel)/scripts

# Create a symbolic link for the pre-commit hook
ln -sfv "$SCRIPTS_DIR/pre-commit" "$HOOKS_DIR/pre-commit"

echo "Git hooks installed successfully."
