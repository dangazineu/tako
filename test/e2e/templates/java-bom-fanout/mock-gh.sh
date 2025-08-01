#!/bin/bash

# Mock GitHub CLI for E2E testing
# Communicates with mock GitHub API server via HTTP

set -e

# Default mock server URL (can be overridden by environment)
GITHUB_API_URL=${GITHUB_API_URL:-"http://localhost:8080"}

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
                # For E2E tests, we'll use a predictable pattern
                owner="${REPO_OWNER:-testorg}"
                repo=$(basename "$(pwd)")
                
                # Create PR via mock API
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
                
                # Output PR URL to match real gh CLI behavior, but ensure PR number is the last line for capture
                echo "https://github.com/$owner/$repo/pull/$pr_number"
                echo "$pr_number"  # This line will be captured by produces.outputs
                ;;
                
            "checks")
                pr_number="$3"
                watch_flag="$4"
                
                owner="${REPO_OWNER:-testorg}"
                repo=$(basename "$(pwd)")
                
                if [ "$watch_flag" = "--watch" ]; then
                    # Blocking watch behavior - poll until CI completes
                    echo "Waiting for CI checks to complete for PR #$pr_number..."
                    
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
                    # Just return current status without watching
                    response=$(make_request "GET" "/repos/$owner/$repo/pulls/$pr_number/checks")
                    echo "$response"
                fi
                ;;
                
            "merge")
                pr_number="$3"
                merge_method="$4"  # --squash, --merge, --rebase
                delete_branch="$5" # --delete-branch
                
                owner="${REPO_OWNER:-testorg}"
                repo=$(basename "$(pwd)")
                
                # Attempt to merge PR
                response=$(make_request "PUT" "/repos/$owner/$repo/pulls/$pr_number/merge" '{"merge_method":"squash"}')
                
                if echo "$response" | grep -q '"merged":true'; then
                    echo "✓ Merged pull request #$pr_number"
                else
                    echo "✗ Failed to merge pull request #$pr_number"
                    echo "$response"
                    exit 1
                fi
                ;;
                
            *)
                echo "Unknown pr subcommand: $subcommand"
                exit 1
                ;;
        esac
        ;;
        
    *)
        echo "Unknown command: $command"
        exit 1
        ;;
esac