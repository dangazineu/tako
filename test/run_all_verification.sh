#!/bin/bash
#
# Wrapper script to run all verification tests based on a manifest file.
# Requires yq to be installed: https://github.com/mikefarah/yq
#
set -euo pipefail

# --- Helper Functions ---
info() {
    echo "[INFO] $1"
}

error() {
    echo "[ERROR] $1" >&2
    exit 1
}

# --- Pre-flight Checks ---
if ! command -v yq &> /dev/null; then
    error "yq is not installed. Please install it to continue. See: https://github.com/mikefarah/yq"
fi

# --- Script Setup ---
# Get the directory of the current script to reliably locate other files.
# The parent directory of this script is the /test directory
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS_DIR="$TEST_DIR/scripts"
MANIFEST="$SCRIPTS_DIR/manifest.yml"

if [ ! -f "$MANIFEST" ]; then
    error "Manifest file not found at: $MANIFEST"
fi

# --- Main Execution ---
info "Starting verification process..."
info "Reading manifest from: $MANIFEST"

# Read the number of scripts from the manifest
script_count=$(yq '.scripts | length' "$MANIFEST")
info "Found $script_count scripts to run."

# Loop through each script in the manifest
for i in $(seq 0 $((script_count - 1))); do
    script_name=$(yq ".scripts[$i].name" "$MANIFEST")
    # Get flags as a space-separated string instead of array for compatibility
    ci_flags_raw=$(yq ".scripts[$i].ci_flags | join(\" \")" "$MANIFEST")
    
    script_path="$SCRIPTS_DIR/$script_name"

    if [ ! -f "$script_path" ]; then
        error "Script '$script_name' defined in manifest not found at '$script_path'"
    fi

    echo
    info "================================================="
    info "Running: $script_name $ci_flags_raw"
    info "================================================="

    # Execute the script from the project root to ensure consistent paths
    # The project root is two levels up from this script's directory
    if [ -n "$ci_flags_raw" ]; then
        (cd "$TEST_DIR/.." && $script_path $ci_flags_raw)
    else
        (cd "$TEST_DIR/.." && $script_path)
    fi

    info "✅ Success: $script_name finished."
done

echo
info "🎉 All verification scripts passed successfully!"