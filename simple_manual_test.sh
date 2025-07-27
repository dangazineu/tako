#!/bin/bash

# Simplified Manual Testing Script for tako/fan-out@v1 Implementation
# This script tests core fan-out functionality using tako CLI

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
TEST_ORG="fanout-test"
TEST_ENV="simple-graph"

# Directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="$(mktemp -d)"
WORK_DIR="$TEST_DIR/work"
CACHE_DIR="$TEST_DIR/cache"

# Binaries
TAKO_BIN="$SCRIPT_DIR/tako"
TAKOTEST_BIN="$SCRIPT_DIR/takotest"

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up test environment...${NC}"
    if [ -n "$CLEANUP_NEEDED" ]; then
        "$TAKOTEST_BIN" cleanup --local --owner "$TEST_ORG" "$TEST_ENV" || true
    fi
    rm -rf "$TEST_DIR" || true
    echo -e "${GREEN}Cleanup completed${NC}"
}

# Set trap for cleanup
trap cleanup EXIT

print_step() {
    echo -e "\n${BLUE}==== $1 ====${NC}"
}

print_substep() {
    echo -e "${YELLOW}-- $1${NC}"
}

verify_success() {
    local description="$1"
    echo -e "${GREEN}✓ $description${NC}"
}

verify_failure() {
    local description="$1"
    echo -e "${RED}✗ $description${NC}"
    return 1
}

# Main test execution
main() {
    echo -e "${BLUE}Simplified Manual Testing Script for tako/fan-out@v1${NC}"
    echo -e "${BLUE}======================================================${NC}"
    
    print_step "Step 1: Build binaries"
    cd "$SCRIPT_DIR"
    go build -o "$TAKO_BIN" ./cmd/tako
    go build -o "$TAKOTEST_BIN" ./cmd/takotest
    
    if [ -x "$TAKO_BIN" ] && [ -x "$TAKOTEST_BIN" ]; then
        verify_success "Binaries built successfully"
    else
        verify_failure "Failed to build binaries"
    fi
    
    print_step "Step 2: Setup test environment"
    CLEANUP_NEEDED=1
    "$TAKOTEST_BIN" setup --local --work-dir "$WORK_DIR" --cache-dir "$CACHE_DIR" --owner "$TEST_ORG" "$TEST_ENV"
    
    if [ -d "$WORK_DIR" ] && [ -d "$CACHE_DIR" ]; then
        verify_success "Test environment created"
    else
        verify_failure "Failed to create test environment"
    fi
    
    print_step "Step 3: Create test workflow with fan-out step"
    
    # Configure the primary repository (repo-a) with fan-out step
    REPO_A_PATH="$WORK_DIR/${TEST_ENV}-repo-a"
    
    cat > "$REPO_A_PATH/tako.yml" << 'EOF'
version: 1

workflows:
  test-fanout:
    inputs:
      version:
        type: string
        required: true
        description: "Version to use for testing"
    steps:
      - id: setup
        run: |
          echo "Setting up test for version {{ .Inputs.version }}"
          echo "test-{{ .Inputs.version }}" > test-output.txt
      
      - id: fanout-test
        uses: tako/fan-out@v1
        with:
          event_type: "test_event"
          wait_for_children: false
      
      - id: cleanup
        run: echo "Cleanup completed"
EOF
    
    # Configure subscriber repository (repo-b) 
    REPO_B_PATH="$CACHE_DIR/repos/$TEST_ORG/${TEST_ENV}-repo-b/main"
    mkdir -p "$REPO_B_PATH"
    
    cat > "$REPO_B_PATH/tako.yml" << 'EOF'
version: 1

subscriptions:
  - artifact: "placeholder/artifact:placeholder"
    events: ["test_event"]
    workflow: "subscriber-workflow"
    inputs:
      test_version: "version"

workflows:
  subscriber-workflow:
    inputs:
      test_version:
        type: string
        required: false
    steps:
      - id: process
        run: |
          echo "Processing event for subscriber workflow"
          echo "subscriber-processed" > subscriber-output.txt
EOF
    
    if [ -f "$REPO_A_PATH/tako.yml" ] && [ -f "$REPO_B_PATH/tako.yml" ]; then
        verify_success "Test configurations created"
    else
        verify_failure "Failed to create test configurations"
    fi
    
    print_step "Step 4: Execute fan-out workflow"
    
    cd "$REPO_A_PATH"
    
    # Test basic fan-out execution
    echo -e "${BLUE}Executing: $TAKO_BIN exec test-fanout --inputs.version=1.0.0 --local --cache-dir $CACHE_DIR --root $REPO_A_PATH${NC}"
    
    if "$TAKO_BIN" exec test-fanout --inputs.version=1.0.0 --local --cache-dir "$CACHE_DIR" --root "$REPO_A_PATH"; then
        verify_success "Fan-out workflow executed successfully"
    else
        verify_failure "Fan-out workflow execution failed"
    fi
    
    print_step "Step 5: Verify results"
    
    # Check that the main workflow ran
    if [ -f "$REPO_A_PATH/test-output.txt" ] && grep -q "test-1.0.0" "$REPO_A_PATH/test-output.txt"; then
        verify_success "Main workflow outputs created correctly"
    else
        verify_failure "Main workflow outputs not found"
    fi
    
    # Check that subscriber workflow actually executed
    if [ -f "$REPO_B_PATH/subscriber-output.txt" ]; then
        if grep -q "subscriber-processed" "$REPO_B_PATH/subscriber-output.txt"; then
            verify_success "Subscriber workflow was actually executed"
        else
            verify_failure "Subscriber workflow output has incorrect content"
        fi
    else
        verify_failure "Subscriber workflow was not executed (no output file found)"
    fi
    
    print_step "Step 6: Test error handling"
    
    # Test workflow with missing required parameter by updating the main config
    cat > "$REPO_A_PATH/tako.yml" << 'EOF'
version: 1

workflows:
  error-test:
    steps:
      - id: invalid-fanout
        uses: tako/fan-out@v1
        with:
          invalid_param: "value"
  
  test-fanout:
    inputs:
      version:
        type: string
        required: true
        description: "Version to use for testing"
    steps:
      - id: setup
        run: |
          echo "Setting up test for version {{ .Inputs.version }}"
          echo "test-{{ .Inputs.version }}" > test-output.txt
      
      - id: fanout-test
        uses: tako/fan-out@v1
        with:
          event_type: "test_event"
          wait_for_children: false
      
      - id: cleanup
        run: echo "Cleanup completed"
EOF

    # This should fail due to missing event_type
    echo -e "${BLUE}Testing error handling (this should fail)${NC}"
    if "$TAKO_BIN" exec error-test --root "$REPO_A_PATH" 2>&1 | grep -q "event_type parameter is required"; then
        verify_success "Error handling works correctly"
    else
        verify_failure "Error handling test did not work as expected"
    fi
    
    print_step "Step 7: Test different fan-out configurations"
    
    # Test with different parameters by updating the config again
    cat > "$REPO_A_PATH/tako.yml" << 'EOF'
version: 1

workflows:
  advanced-fanout:
    steps:
      - id: full-config-fanout
        uses: tako/fan-out@v1
        with:
          event_type: "advanced_event"
          wait_for_children: true
          timeout: "5m"
          concurrency_limit: 2
EOF

    echo -e "${BLUE}Testing advanced fan-out configuration${NC}"
    if "$TAKO_BIN" exec advanced-fanout --root "$REPO_A_PATH" --local --cache-dir "$CACHE_DIR"; then
        verify_success "Advanced fan-out configuration works"
    else
        verify_failure "Advanced fan-out configuration failed"
    fi
    
    print_step "Summary: Manual Testing Results"
    
    echo -e "${GREEN}"
    echo "✓ Fan-out step executes successfully"
    echo "✓ Child workflows are actually triggered and executed" 
    echo "✓ Parameter validation works correctly" 
    echo "✓ Error handling functions as expected"
    echo "✓ Cross-repository orchestration is functional"
    echo "✓ Integration with tako workflow system successful"
    echo -e "${NC}"
    
    echo -e "${BLUE}Manual testing completed successfully!${NC}"
    echo -e "${BLUE}The tako/fan-out@v1 step is functional and ready for use.${NC}"
    
    return 0
}

# Execute main function
main "$@"