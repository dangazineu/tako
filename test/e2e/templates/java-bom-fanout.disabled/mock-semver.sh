#!/bin/bash

# Mock semver tool for E2E testing
# Simulates semantic version operations

set -e

# Parse command line arguments
flag="$1"
increment_type="$2"
base_version="$3"

case "$flag" in
    "-i")
        
        # Extract version parts
        if [[ "$base_version" =~ ^v?([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
            major="${BASH_REMATCH[1]}"
            minor="${BASH_REMATCH[2]}"
            patch="${BASH_REMATCH[3]}"
        else
            # Default version if parsing fails
            major=1
            minor=0
            patch=0
        fi
        
        case "$increment_type" in
            "major")
                major=$((major + 1))
                minor=0
                patch=0
                ;;
            "minor")
                minor=$((minor + 1))
                patch=0
                ;;
            "patch")
                patch=$((patch + 1))
                ;;
            *)
                echo "Unknown increment type: $increment_type"
                exit 1
                ;;
        esac
        
        echo "${major}.${minor}.${patch}"
        ;;
        
    *)
        echo "Unknown semver flag: $flag"
        exit 1
        ;;
esac