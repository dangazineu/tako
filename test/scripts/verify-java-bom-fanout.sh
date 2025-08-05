#!/bin/bash
set -e

# Script to verify the Java BOM fanout orchestration functionality
echo "=== Tako Java BOM Fanout Orchestration Verification Script ==="
echo

# Parse command line arguments
PRESERVE_TEST_DIR=false
TEST_ENVIRONMENT="java-bom-fanout"
LOCAL_MODE=true
REMOTE_MODE=false
OWNER="tako-test"
while [[ $# -gt 0 ]]; do
    case $1 in
        --preserve-test-dir)
            PRESERVE_TEST_DIR=true
            shift
            ;;
        --test-env)
            TEST_ENVIRONMENT="$2"
            shift 2
            ;;
        --owner)
            OWNER="$2"
            shift 2
            ;;
        --remote)
            REMOTE_MODE=true
            LOCAL_MODE=false
            shift
            ;;
        --local)
            LOCAL_MODE=true
            REMOTE_MODE=false
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  --preserve-test-dir    Do not delete test directories when script completes"
            echo "  --test-env ENV         Use specific test environment (default: java-bom-fanout)"
            echo "  --owner OWNER          GitHub organization or user (default: tako-test)"
            echo "  --local                Use local mode (default)"
            echo "  --remote               Use remote mode (requires GitHub token)"
            echo "  --help, -h             Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print status
print_status() {
    local status=$1
    local message=$2
    
    if [ "$status" = "PASS" ]; then
        echo -e "${GREEN}✓${NC} $message"
    elif [ "$status" = "FAIL" ]; then
        echo -e "${RED}✗${NC} $message"
    else
        echo -e "${YELLOW}•${NC} $message"
    fi
}

# Function to cleanup test directory
cleanup_test_dir() {
    if [ "$PRESERVE_TEST_DIR" = "false" ] && [ -n "$TEST_BASE_DIR" ] && [ -d "$TEST_BASE_DIR" ]; then
        echo "Cleaning up test directory: $TEST_BASE_DIR"
        rm -rf "$TEST_BASE_DIR"
    elif [ "$PRESERVE_TEST_DIR" = "true" ] && [ -n "$TEST_BASE_DIR" ]; then
        echo "Test directory preserved at: $TEST_BASE_DIR"
    fi
}

# Set up cleanup trap
trap cleanup_test_dir EXIT

# Set up environment for PR state management (will be created when TEST_BASE_DIR is set)
# export PR_STATE_DIR will be set after TEST_BASE_DIR is defined

# For E2E tests, set E2E_MODE to true to skip human intervention
if [ "${E2E_MODE:-}" = "true" ]; then
    export E2E_MODE="true"
    echo "Running in E2E mode - automated PR handling"
else
    export E2E_MODE="false"
    echo "Running in verification mode - manual PR intervention required"
fi

# Get the script directory and project root
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/../.." &> /dev/null && pwd )"

echo "Project root: $PROJECT_ROOT"
echo "Using test environment: $TEST_ENVIRONMENT"
echo "Owner: $OWNER"
if [ "$LOCAL_MODE" = "true" ]; then
    echo "Mode: Local"
else
    echo "Mode: Remote (requires GitHub token)"
fi

# Change to project root
cd "$PROJECT_ROOT"

# Build tako from local source
echo "Building tako from local source..."
if ! go build -o ./tako ./cmd/tako; then
    echo -e "${RED}Error: Failed to build tako from local source${NC}"
    exit 1
fi
print_status "PASS" "Built tako from local source"

# Build takotest from local source
echo "Building takotest from local source..."
if ! go build -o ./takotest ./cmd/takotest; then
    echo -e "${RED}Error: Failed to build takotest from local source${NC}"
    exit 1
fi
print_status "PASS" "Built takotest from local source"

# Create test directory in the local project
TEST_BASE_DIR="$PROJECT_ROOT/test-java-bom-fanout-$(date +%s)"
TEST_DIR="$TEST_BASE_DIR/work"
CACHE_DIR="$TEST_BASE_DIR/cache"

echo "Creating test environment in: $TEST_BASE_DIR"
mkdir -p "$TEST_DIR" "$CACHE_DIR"

# Set up environment for PR state management
export PR_STATE_DIR="$TEST_BASE_DIR/pr-state"
mkdir -p "$PR_STATE_DIR"

# Set up test environment using takotest
echo "Setting up test environment using takotest..."
if [ "$LOCAL_MODE" = "true" ]; then
    TAKOTEST_OUTPUT=$("$PROJECT_ROOT/takotest" setup --local --work-dir "$TEST_DIR" --cache-dir "$CACHE_DIR" --owner "$OWNER" "$TEST_ENVIRONMENT" 2>&1)
else
    TAKOTEST_OUTPUT=$("$PROJECT_ROOT/takotest" setup --with-repo-entrypoint --work-dir "$TEST_DIR" --cache-dir "$CACHE_DIR" --owner "$OWNER" "$TEST_ENVIRONMENT" 2>&1)
fi

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to set up test environment with takotest${NC}"
    echo "$TAKOTEST_OUTPUT"
    exit 1
fi

echo "$TAKOTEST_OUTPUT"
print_status "PASS" "Test environment set up with takotest"

# Change to the test working directory
cd "$TEST_DIR"

# Use the locally built tako for all operations
TAKO_CMD="$PROJECT_ROOT/tako"

echo
echo "=== Running Verification Tests ==="
echo

# Test 1: Validate configurations
echo "Test 1: Validating tako configurations..."
# List actual repositories created by takotest
echo "Repositories created:"
ls -la
echo

# Find the actual repo directories 
if [ "$LOCAL_MODE" = "true" ]; then
    # In local mode, repositories are in work directory
    REPO_DIRS=($(find . -maxdepth 1 -type d -name "*java-bom-fanout*" | sed 's|./||'))
else
    # In remote mode with --with-repo-entrypoint, repositories are in cache directory
    REPO_DIRS=($(find "$CACHE_DIR" -path "*/repos/*/java-bom-fanout*/main" -type d | grep -v "src/main"))
fi

if [ ${#REPO_DIRS[@]} -eq 0 ]; then
    print_status "FAIL" "No test repositories found"
    if [ "$LOCAL_MODE" != "true" ]; then
        echo "Cache directory structure:"
        find "$CACHE_DIR" -type d -name "*java-bom-fanout*" 2>/dev/null || echo "No cache directories found"
    fi
    exit 1
else
    for repo in "${REPO_DIRS[@]}"; do
        if [ "$LOCAL_MODE" = "true" ]; then
            repo_path="$repo"
        else
            repo_path="$repo"  # Already full path from find command
        fi
        
        if "$TAKO_CMD" validate --root "$repo_path" &> /dev/null; then
            print_status "PASS" "Configuration valid for $(basename $repo_path)"
        else
            print_status "FAIL" "Configuration invalid for $(basename $repo_path)"
            "$TAKO_CMD" validate --root "$repo_path"
            exit 1
        fi
    done
fi

# Test 2: Find orchestrator repository
echo
echo "Test 2: Locating orchestrator repository..."
ORCHESTRATOR_REPO=""
if [ "$LOCAL_MODE" = "true" ]; then
    # In local mode, look for orchestrator in cache with main branch
    ORCHESTRATOR_REPO=$(find "$CACHE_DIR" -path "*orchestrator*/main" -type d | head -1)
    if [ -z "$ORCHESTRATOR_REPO" ]; then
        print_status "FAIL" "Orchestrator repository not found in cache"
        echo "Available directories in cache:"
        find "$CACHE_DIR" -type d -name "*java-bom-fanout*" || true
        exit 1
    fi
else
    # In remote mode, orchestrator should be accessible via --repo flag
    ORCHESTRATOR_REPO="$OWNER/java-bom-fanout-java-bom-fanout-orchestrator"
fi
print_status "PASS" "Found orchestrator repository: $ORCHESTRATOR_REPO"

# Test 3: Dry run to see execution plan
echo
echo "Test 3: Testing dry-run execution..."
if [ "$LOCAL_MODE" = "true" ]; then
    if "$TAKO_CMD" exec release-train --root "$ORCHESTRATOR_REPO" --inputs.version=1.0.0 --dry-run --cache-dir "$CACHE_DIR" &> /dev/null; then
        print_status "PASS" "Dry-run execution successful"
    else
        print_status "FAIL" "Dry-run execution failed"
        "$TAKO_CMD" exec release-train --root "$ORCHESTRATOR_REPO" --inputs.version=1.0.0 --dry-run --cache-dir "$CACHE_DIR"
        exit 1
    fi
else
    if "$TAKO_CMD" exec release-train --repo "$ORCHESTRATOR_REPO" --inputs.version=1.0.0 --dry-run --cache-dir "$CACHE_DIR" &> /dev/null; then
        print_status "PASS" "Dry-run execution successful"
    else
        print_status "FAIL" "Dry-run execution failed"
        "$TAKO_CMD" exec release-train --repo "$ORCHESTRATOR_REPO" --inputs.version=1.0.0 --dry-run --cache-dir "$CACHE_DIR"
        exit 1
    fi
fi

# Test 4: Execute the orchestrated release train
echo
echo "Test 4: Executing orchestrated release train..."
EXECUTION_OUTPUT=""

# Export environment variables for the orchestrator workflow
export REPO_OWNER="$OWNER"
export MAVEN_REPO_DIR="$TEST_BASE_DIR/maven-repo"
export TAKO_BINARY="$TAKO_CMD"
export CACHE_DIR="$CACHE_DIR"
mkdir -p "$MAVEN_REPO_DIR"

# Function to handle PR intervention during workflow execution
handle_pr_intervention() {
    local orchestrator_pid=$1
    
    if [ "$E2E_MODE" = "true" ]; then
        # In E2E mode, just wait for the process to complete
        wait $orchestrator_pid
        return $?
    fi
    
    # In manual mode, monitor for PR prompts and handle them
    echo "Monitoring for PR merge prompts..."
    echo "Note: When prompted, you can choose to auto-merge PRs or handle them manually"
    
    # Wait for the orchestrator process to complete
    wait $orchestrator_pid
    return $?
}

if [ "$LOCAL_MODE" = "true" ]; then
    # Start the orchestrator in the background to handle potential prompts
    if [ "$E2E_MODE" = "true" ]; then
        # E2E mode: run directly with automated handling
        EXECUTION_OUTPUT=$("$TAKO_CMD" exec release-train --root "$ORCHESTRATOR_REPO" --inputs.version=1.2.0 --cache-dir "$CACHE_DIR" 2>&1)
        EXECUTION_EXIT_CODE=$?
    else
        # Manual verification mode: run with potential for user interaction
        echo "Starting release train execution (may require manual PR intervention)..."
        EXECUTION_OUTPUT=$("$TAKO_CMD" exec release-train --root "$ORCHESTRATOR_REPO" --inputs.version=1.2.0 --cache-dir "$CACHE_DIR" 2>&1)
        EXECUTION_EXIT_CODE=$?
    fi
else
    # Remote mode
    if [ "$E2E_MODE" = "true" ]; then
        EXECUTION_OUTPUT=$("$TAKO_CMD" exec release-train --repo "$ORCHESTRATOR_REPO" --inputs.version=1.2.0 --cache-dir "$CACHE_DIR" 2>&1)
        EXECUTION_EXIT_CODE=$?
    else
        echo "Starting release train execution in remote mode (may require manual PR intervention)..."
        EXECUTION_OUTPUT=$("$TAKO_CMD" exec release-train --repo "$ORCHESTRATOR_REPO" --inputs.version=1.2.0 --cache-dir "$CACHE_DIR" 2>&1)
        EXECUTION_EXIT_CODE=$?
    fi
fi

if [ $EXECUTION_EXIT_CODE -eq 0 ]; then
    print_status "PASS" "Release train execution completed successfully"
else
    print_status "FAIL" "Release train execution failed"
    echo "Output:"
    echo "$EXECUTION_OUTPUT"
    exit 1
fi

# Test 5: Verify orchestration steps were executed
echo
echo "Test 5: Verifying orchestration steps..."
expected_steps=("start-release-train" "release-core-lib" "trigger-downstream-updates" "verify-release-train")
for step in "${expected_steps[@]}"; do
    if echo "$EXECUTION_OUTPUT" | grep -q "$step"; then
        print_status "PASS" "Step '$step' was executed"
    else
        print_status "FAIL" "Step '$step' was not found in output"
        echo "Full output:"
        echo "$EXECUTION_OUTPUT"
        exit 1
    fi
done

# Test 6: Verify success indicators in output
echo
echo "Test 6: Verifying success indicators..."
success_indicators=("Success: true" "Steps executed: 4" "Execution completed:")
for indicator in "${success_indicators[@]}"; do
    if echo "$EXECUTION_OUTPUT" | grep -q "$indicator"; then
        print_status "PASS" "Found success indicator: '$indicator'"
    else
        print_status "FAIL" "Missing success indicator: '$indicator'"
        echo "Full output:"
        echo "$EXECUTION_OUTPUT"
        exit 1
    fi
done

# Test 7: Verify created files (local mode only)
if [ "$LOCAL_MODE" = "true" ]; then
    echo
    echo "Test 7: Verifying created files in orchestrator workspace..."
    
    # Check for expected files in orchestrator directory
    expected_files=("published_core-lib_1.2.0.txt" "core-lib-version.txt")
    file_checks_passed=0
    
    for file in "${expected_files[@]}"; do
        if [ -f "$ORCHESTRATOR_REPO/$file" ]; then
            print_status "PASS" "Found expected file: $file"
            file_checks_passed=$((file_checks_passed + 1))
        else
            print_status "FAIL" "Missing expected file: $file"
        fi
    done
    
    # Check for wildcard files (with timestamps)
    wildcard_files=("published_lib-a_*.txt" "published_lib-b_*.txt" "published_java-bom_*.txt" "final_bom_state_*.json")
    for pattern in "${wildcard_files[@]}"; do
        if ls "$ORCHESTRATOR_REPO"/$pattern 1> /dev/null 2>&1; then
            print_status "PASS" "Found expected file pattern: $pattern"
            file_checks_passed=$((file_checks_passed + 1))
        else
            print_status "FAIL" "Missing expected file pattern: $pattern"
        fi
    done
    
    if [ $file_checks_passed -lt 4 ]; then
        echo "Files found in orchestrator directory:"
        ls -la "$ORCHESTRATOR_REPO" || true
        exit 1
    fi
fi

# Test 8: Test error handling with invalid input
echo
echo "Test 8: Testing error handling with missing required input..."
ERROR_OUTPUT=""
if [ "$LOCAL_MODE" = "true" ]; then
    ERROR_OUTPUT=$("$TAKO_CMD" exec release-train --root "$ORCHESTRATOR_REPO" --cache-dir "$CACHE_DIR" 2>&1 || true)
else
    ERROR_OUTPUT=$("$TAKO_CMD" exec release-train --repo "$ORCHESTRATOR_REPO" --cache-dir "$CACHE_DIR" 2>&1 || true)
fi

if echo "$ERROR_OUTPUT" | grep -q -i "required input.*version.*not provided"; then
    print_status "PASS" "Error handling works correctly for missing inputs"
elif echo "$ERROR_OUTPUT" | grep -q -i "required"; then
    print_status "PASS" "Error handling works correctly (alternative error message)"
else
    print_status "FAIL" "Error handling did not work as expected"
    echo "Error output:"
    echo "$ERROR_OUTPUT"
    exit 1
fi

echo
echo "=== All Verification Tests Passed! ==="
echo
print_status "PASS" "Java BOM fanout orchestration is working correctly"

if [ "$PRESERVE_TEST_DIR" = "true" ]; then
    echo
    echo "Test artifacts preserved at: $TEST_BASE_DIR"
    echo "To clean up manually, run: rm -rf $TEST_BASE_DIR"
fi