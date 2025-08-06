#!/bin/bash

# Mock GitHub CLI for E2E testing
# Communicates with mock GitHub API server via HTTP or simulates locally

set -e

# Default mock server URL (can be overridden by environment)
GITHUB_API_URL=${GITHUB_API_URL:-"http://localhost:8080"}

# Check if we're in automated E2E mode or manual verification mode
E2E_MODE=${E2E_MODE:-"false"}
PR_STATE_DIR=${PR_STATE_DIR:-"/tmp/tako-pr-state"}

# Create state directory if it doesn't exist
mkdir -p "$PR_STATE_DIR"

# Function to make HTTP requests to mock server
make_request() {
    local method="$1"
    local endpoint="$2"
    local data="$3"
    
    if [ -n "$data" ]; then
        curl -s -X "$method" \
             -H "Content-Type: application/json" \
             -d "$data" \
             "$GITHUB_API_URL$endpoint"
    else
        curl -s -X "$method" "$GITHUB_API_URL$endpoint"
    fi
}

# Function to prompt for user action (for manual verification mode)
prompt_for_merge() {
    local pr_number="$1"
    local repo="$2"
    
    if [ "$E2E_MODE" = "true" ]; then
        # In E2E mode, automatically mark as merged after simulated CI
        echo "✓ [E2E Mode] Automatically merging PR #$pr_number"
        echo "merged" > "$PR_STATE_DIR/${repo}_pr_${pr_number}.state"
        return 0
    fi
    
    # In manual mode, prompt the user
    echo ""
    echo "======================================"
    echo "MANUAL INTERVENTION REQUIRED"
    echo "======================================"
    echo "PR #$pr_number in $repo needs to be merged"
    echo ""
    echo "Options:"
    echo "  1) Auto-merge (simulate merge in local mode)"
    echo "  2) Continue (assume PR was manually merged)"
    echo "  3) Exit"
    echo ""
    read -p "Enter choice [1-3]: " choice
    
    case $choice in
        1)
            echo "merged" > "$PR_STATE_DIR/${repo}_pr_${pr_number}.state"
            echo "✓ PR #$pr_number marked as merged (simulated)"
            return 0
            ;;
        2)
            # Check if actually merged (in remote mode this would check GitHub)
            if [ -f "$PR_STATE_DIR/${repo}_pr_${pr_number}.state" ]; then
                local state=$(cat "$PR_STATE_DIR/${repo}_pr_${pr_number}.state")
                if [ "$state" = "merged" ]; then
                    echo "✓ PR #$pr_number is merged"
                    return 0
                fi
            fi
            echo "⚠ PR #$pr_number is not yet merged. Please merge it first."
            return 1
            ;;
        3)
            echo "Exiting..."
            exit 1
            ;;
        *)
            echo "Invalid choice"
            return 1
            ;;
    esac
}

# Parse command line arguments
command="$1"
subcommand="$2"

case "$command" in
    "pr")
        case "$subcommand" in
            "create")
                # Parse gh pr create arguments
                title=""
                body=""
                head="feature-branch"
                base="main"
                
                while [[ $# -gt 0 ]]; do
                    case $1 in
                        --title)
                            title="$2"
                            shift 2
                            ;;
                        --body)
                            body="$2"
                            shift 2
                            ;;
                        --head)
                            head="$2"
                            shift 2
                            ;;
                        --base)
                            base="$2"
                            shift 2
                            ;;
                        *)
                            shift
                            ;;
                    esac
                done
                
                # Extract owner/repo from current directory or environment
                owner="${REPO_OWNER:-tako-test}"
                repo=$(basename "$(pwd)")
                
                # Generate a simple PR number based on timestamp
                pr_number=$(($(date +%s) % 10000))
                
                # Store PR state
                echo "open" > "$PR_STATE_DIR/${repo}_pr_${pr_number}.state"
                echo "$title" > "$PR_STATE_DIR/${repo}_pr_${pr_number}.title"
                
                # Try to create PR via mock API if available
                if curl -s -f "$GITHUB_API_URL/health" > /dev/null 2>&1; then
                    create_data=$(cat <<EOF
{
    "title": "$title",
    "body": "$body", 
    "head": "$head",
    "base": "$base"
}
EOF
)
                    response=$(make_request "POST" "/repos/$owner/$repo/pulls" "$create_data")
                    pr_number=$(echo "$response" | grep -o '"number":[0-9]*' | cut -d':' -f2)
                fi
                
                # Output PR URL to match real gh CLI behavior, but ensure PR number is the last line for capture
                echo "Creating pull request #$pr_number for $repo"
                echo "Title: $title"
                echo "https://github.com/$owner/$repo/pull/$pr_number"
                echo "$pr_number"  # This line will be captured by produces.outputs
                ;;
                
            "checks")
                pr_number="$3"
                watch_flag="$4"
                
                owner="${REPO_OWNER:-tako-test}"
                repo=$(basename "$(pwd)")
                
                if [ "$watch_flag" = "--watch" ]; then
                    # Blocking watch behavior
                    echo "Waiting for CI checks to complete for PR #$pr_number..."
                    
                    if [ "$E2E_MODE" = "true" ]; then
                        # In E2E mode, simulate quick CI completion
                        sleep 2
                        echo "✓ CI checks passed (E2E simulation)"
                        exit 0
                    fi
                    
                    # Check if we can connect to mock server
                    if curl -s -f "$GITHUB_API_URL/health" > /dev/null 2>&1; then
                        # Poll mock server for CI status
                        while true; do
                            response=$(make_request "GET" "/repos/$owner/$repo/pulls/$pr_number/checks")
                            status=$(echo "$response" | grep -o '"status":"[^"]*' | cut -d'"' -f4)
                            conclusion=$(echo "$response" | grep -o '"conclusion":"[^"]*' | cut -d'"' -f4)
                            
                            if [ "$status" = "completed" ]; then
                                if [ "$conclusion" = "success" ]; then
                                    echo "✓ CI checks passed"
                                    exit 0
                                elif [ "$conclusion" = "failure" ]; then
                                    echo "✗ CI checks failed"
                                    exit 1
                                fi
                            fi
                            
                            echo "  Still waiting for checks... (status: $status)"
                            sleep 2
                        done
                    else
                        # Simulate CI completion in local mode
                        echo "  Simulating CI checks..."
                        sleep 3
                        echo "✓ CI checks passed (simulated)"
                        exit 0
                    fi
                else
                    # Just return current status without watching
                    echo '{"status":"completed","conclusion":"success"}'
                fi
                ;;
                
            "merge")
                pr_number="$3"
                merge_method="$4"  # --squash, --merge, --rebase
                delete_branch="$5" # --delete-branch
                
                owner="${REPO_OWNER:-tako-test}"
                repo=$(basename "$(pwd)")
                
                # Check if PR is marked as merged
                if [ -f "$PR_STATE_DIR/${repo}_pr_${pr_number}.state" ]; then
                    state=$(cat "$PR_STATE_DIR/${repo}_pr_${pr_number}.state")
                    if [ "$state" != "merged" ]; then
                        # Prompt for merge if not in E2E mode
                        if ! prompt_for_merge "$pr_number" "$repo"; then
                            echo "✗ PR #$pr_number is not merged"
                            exit 1
                        fi
                    fi
                fi
                
                # Try to merge via mock API if available
                if curl -s -f "$GITHUB_API_URL/health" > /dev/null 2>&1; then
                    response=$(make_request "PUT" "/repos/$owner/$repo/pulls/$pr_number/merge" '{"merge_method":"squash"}')
                    
                    if echo "$response" | grep -q '"merged":true'; then
                        echo "✓ Merged pull request #$pr_number"
                    else
                        echo "✗ Failed to merge pull request #$pr_number"
                        echo "$response"
                        exit 1
                    fi
                else
                    # Local simulation
                    echo "✓ Merged pull request #$pr_number (simulated)"
                    echo "merged" > "$PR_STATE_DIR/${repo}_pr_${pr_number}.state"
                fi
                ;;
                
            *)
                echo "Unknown pr subcommand: $subcommand"
                exit 1
                ;;
        esac
        ;;
        
    "release")
        case "$subcommand" in
            "create")
                # Parse gh release create arguments
                tag=""
                title=""
                notes=""
                target="main"
                
                shift 2  # Skip "release" and "create"
                tag="$1"
                shift
                
                while [[ $# -gt 0 ]]; do
                    case $1 in
                        --title)
                            title="$2"
                            shift 2
                            ;;
                        --notes)
                            notes="$2"
                            shift 2
                            ;;
                        --target)
                            target="$2"
                            shift 2
                            ;;
                        *)
                            shift
                            ;;
                    esac
                done
                
                owner="${REPO_OWNER:-tako-test}"
                repo=$(basename "$(pwd)")
                
                # For local mode, just simulate the release creation
                echo "Creating release $tag for $owner/$repo"
                echo "Title: $title"
                echo "Notes: $notes"
                echo "Target: $target"
                
                # Create a marker file for E2E testing
                echo "$tag" > "release-$tag.txt"
                
                echo "✓ Release $tag created successfully"
                echo "https://github.com/$owner/$repo/releases/tag/$tag"
                ;;
                
            *)
                echo "Unknown release subcommand: $subcommand"
                exit 1
                ;;
        esac
        ;;
        
    *)
        echo "Unknown command: $command"
        exit 1
        ;;
esac