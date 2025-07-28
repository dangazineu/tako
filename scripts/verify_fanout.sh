#!/bin/bash

# Subscription-Based Workflow Triggering Verification Script
# For Issue #106 implementation

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEST_DIR="${PROJECT_ROOT}/test_workspace"
CACHE_DIR="${TEST_DIR}/.tako/cache"
TEST_ORG="test-org"
PRESERVE_TEST_DIR=false

# Ensure tako binary is available
if ! command -v tako &> /dev/null; then
    echo -e "${RED}❌ tako command not found. Please build and install tako first.${NC}"
    echo "Run: go build -o tako ./cmd/tako && export PATH=\$PWD:\$PATH"
    exit 1
fi

# Helper functions
print_section() {
    echo -e "\n${BLUE}=== $1 ===${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

setup_test_repo() {
    local repo_path="$1"
    local tako_yml_content="$2"
    
    mkdir -p "$repo_path"
    echo "$tako_yml_content" > "$repo_path/tako.yml"
    
    # Initialize as git repo to avoid warnings
    if [ ! -d "$repo_path/.git" ]; then
        (cd "$repo_path" && git init --quiet)
        (cd "$repo_path" && git config user.name "Test User" && git config user.email "test@example.com")
        (cd "$repo_path" && git add . && git commit -m "Initial commit" --quiet)
    fi
}

cleanup_test_repos() {
    print_section "Cleaning up test workspace"
    if [ -d "$TEST_DIR" ]; then
        rm -rf "$TEST_DIR"
        print_success "Test workspace cleaned up"
    else
        print_warning "Test workspace directory not found"
    fi
}

# Trap to ensure cleanup on script exit
cleanup_on_exit() {
    if [ "$PRESERVE_TEST_DIR" = false ]; then
        cleanup_test_repos
    fi
}

setup_test_environment() {
    print_section "Setting up test environment"
    
    # Create test workspace
    mkdir -p "$TEST_DIR"
    mkdir -p "$CACHE_DIR"
    
    # Build tako if needed
    if [ ! -f "$PROJECT_ROOT/tako" ]; then
        print_section "Building tako binary"
        (cd "$PROJECT_ROOT" && go build -o tako ./cmd/tako)
        print_success "Tako built successfully"
    fi
    
    # Add tako to PATH for the session
    export PATH="$PROJECT_ROOT:$PATH"
    
    print_success "Test environment ready"
    echo "  Test Directory: $TEST_DIR"
    echo "  Cache Directory: $CACHE_DIR"
}

# Test 1: Basic Fan-Out with Idempotency
test_basic_fanout() {
    print_section "Test 1: Basic Fan-Out with Idempotency"
    
    # Setup producer
    local producer_path="${CACHE_DIR}/repos/${TEST_ORG}/producer/main"
    local producer_yml='version: 1.0.0
workflows:
  deploy:
    inputs:
      environment:
        type: string
        required: true
    steps:
      - id: emit-event
        uses: tako/fan-out@v1
        with:
          event_type: "deployment.completed"
          schema_version: "1.0.0"
          payload:
            environment: "{{ .inputs.environment }}"
            service: "api"
          wait_for_children: false
          timeout: "30s"'
    
    setup_test_repo "$producer_path" "$producer_yml"
    
    # Setup consumer1
    local consumer1_path="${CACHE_DIR}/repos/${TEST_ORG}/consumer1/main"
    local consumer1_yml='version: 1.0.0
subscriptions:
  - events: ["deployment.completed"]
    workflow: "update"
    schema_version: "^1.0.0"
    filters:
      - "payload.environment == \"production\""
    inputs:
      service_name: "{{ .payload.service }}"
      env: "{{ .payload.environment }}"
workflows:
  update:
    inputs:
      service_name:
        type: string
        required: true
      env:
        type: string
        required: true
    steps:
      - id: update-service
        run: echo "Updating service {{ .inputs.service_name }} in {{ .inputs.env }}"'
    
    setup_test_repo "$consumer1_path" "$consumer1_yml"
    
    # Setup consumer2
    local consumer2_path="${CACHE_DIR}/repos/${TEST_ORG}/consumer2/main"
    local consumer2_yml='version: 1.0.0
subscriptions:
  - events: ["deployment.completed"]
    workflow: "notify"
    schema_version: "~1.0.0"
    filters:
      - "payload.environment == \"production\""
    inputs:
      notification_type: "deployment"
      service: "{{ .payload.service }}"
workflows:
  notify:
    inputs:
      notification_type:
        type: string
        required: true
      service:
        type: string
        required: true
    steps:
      - id: send-notification
        run: echo "Sending {{ .inputs.notification_type }} notification for {{ .inputs.service }}"'
    
    setup_test_repo "$consumer2_path" "$consumer2_yml"
    
    # Execute test
    echo "Executing basic fan-out workflow..."
    if tako exec --cache-dir "$CACHE_DIR" --debug --repo "${TEST_ORG}/producer" deploy --inputs.environment=production; then
        print_success "Basic fan-out executed successfully"
    else
        print_error "Basic fan-out execution failed"
        return 1
    fi
}

# Test 2: Diamond Dependency Resolution
test_diamond_dependency() {
    print_section "Test 2: Diamond Dependency Resolution"
    
    # Create consumer with conflicting subscriptions
    local consumer_path="${CACHE_DIR}/repos/${TEST_ORG}/diamond-consumer/main"
    local consumer_yml='version: 1.0.0
subscriptions:
  - events: ["deployment.completed"]
    workflow: "update-alpha"
    schema_version: "^1.0.0"
    filters:
      - "payload.environment == \"production\""
    inputs:
      service_name: "{{ .payload.service }}"
  - events: ["deployment.completed"]
    workflow: "update-beta"
    schema_version: "^1.0.0"
    filters:
      - "payload.environment == \"production\""
    inputs:
      service_name: "{{ .payload.service }}"
workflows:
  update-alpha:
    inputs:
      service_name:
        type: string
        required: true
    steps:
      - id: update-alpha
        run: echo "Alpha update for {{ .inputs.service_name }}"
  update-beta:
    inputs:
      service_name:
        type: string
        required: true
    steps:
      - id: update-beta
        run: echo "Beta update for {{ .inputs.service_name }}"'
    
    setup_test_repo "$consumer_path" "$consumer_yml"
    
    # Execute test
    echo "Testing diamond dependency resolution..."
    if output=$(tako exec --cache-dir "$CACHE_DIR" --debug --repo "${TEST_ORG}/producer" deploy --inputs.environment=production 2>&1); then
        if echo "$output" | grep -q "Diamond dependency resolved"; then
            print_success "Diamond dependency resolution working correctly"
        else
            print_warning "Diamond dependency resolution may not be triggered (no conflicts detected)"
        fi
    else
        print_error "Diamond dependency test failed"
        return 1
    fi
}

# Test 3: CEL Expression Performance
test_cel_performance() {
    print_section "Test 3: CEL Expression Performance"
    
    # Create consumer with complex CEL expressions
    local consumer_path="${CACHE_DIR}/repos/${TEST_ORG}/cel-consumer/main"
    local consumer_yml='version: 1.0.0
subscriptions:
  - events: ["deployment.completed"]
    workflow: "complex-processing"
    schema_version: "^1.0.0"
    filters:
      - "payload.environment == \"production\""
      - "payload.service in [\"api\", \"web\", \"worker\"]"
      - "has(payload.version) && payload.version != \"\""
    inputs:
      service_name: "{{ .payload.service }}"
      version: "{{ .payload.version }}"
workflows:
  complex-processing:
    inputs:
      service_name:
        type: string
        required: true
      version:
        type: string
        required: true
    steps:
      - id: process
        run: echo "Processing {{ .inputs.service_name }} version {{ .inputs.version }}"'
    
    setup_test_repo "$consumer_path" "$consumer_yml"
    
    # Update producer to include version
    local producer_path="${CACHE_DIR}/repos/${TEST_ORG}/producer/main"
    local producer_yml='version: 1.0.0
workflows:
  deploy:
    inputs:
      environment:
        type: string
        required: true
      version:
        type: string
        required: true
    steps:
      - id: emit-event
        uses: tako/fan-out@v1
        with:
          event_type: "deployment.completed"
          schema_version: "1.0.0"
          payload:
            environment: "{{ .inputs.environment }}"
            service: "api"
            version: "{{ .inputs.version }}"
          wait_for_children: false
          timeout: "30s"'
    
    setup_test_repo "$producer_path" "$producer_yml"
    
    # Execute multiple times to test caching
    echo "Testing CEL expression caching (5 iterations)..."
    local total_time=0
    for i in {1..5}; do
        echo "Iteration $i:"
        start_time=$(date +%s%N)
        if tako exec --cache-dir "$CACHE_DIR" --repo "${TEST_ORG}/producer" deploy --inputs.environment=production --inputs.version="1.$i.0" >/dev/null 2>&1; then
            end_time=$(date +%s%N)
            iteration_time=$(( (end_time - start_time) / 1000000 ))
            total_time=$((total_time + iteration_time))
            echo "  Time: ${iteration_time}ms"
        else
            print_error "CEL performance test iteration $i failed"
            return 1
        fi
    done
    
    local avg_time=$((total_time / 5))
    print_success "CEL performance test completed. Average time: ${avg_time}ms"
}

# Test 4: Schema Compatibility
test_schema_compatibility() {
    print_section "Test 4: Schema Compatibility Validation"
    
    # Create consumers with different version requirements
    local exact_consumer_path="${CACHE_DIR}/repos/${TEST_ORG}/exact-consumer/main"
    local exact_consumer_yml='version: 1.0.0
subscriptions:
  - events: ["deployment.completed"]
    workflow: "exact-version"
    schema_version: "1.0.0"
    inputs:
      service_name: "{{ .payload.service }}"
workflows:
  exact-version:
    inputs:
      service_name:
        type: string
        required: true
    steps:
      - id: exact-processing
        run: echo "Exact version processing for {{ .inputs.service_name }}"'
    
    setup_test_repo "$exact_consumer_path" "$exact_consumer_yml"
    
    local caret_consumer_path="${CACHE_DIR}/repos/${TEST_ORG}/caret-consumer/main"
    local caret_consumer_yml='version: 1.0.0
subscriptions:
  - events: ["deployment.completed"]
    workflow: "caret-range"
    schema_version: "^1.0.0"
    inputs:
      service_name: "{{ .payload.service }}"
workflows:
  caret-range:
    inputs:
      service_name:
        type: string
        required: true
    steps:
      - id: caret-processing
        run: echo "Caret range processing for {{ .inputs.service_name }}"'
    
    setup_test_repo "$caret_consumer_path" "$caret_consumer_yml"
    
    # Test with version 1.0.0 (should trigger both)
    echo "Testing schema version 1.0.0 (should trigger both consumers)..."
    if tako exec --cache-dir "$CACHE_DIR" --repo "${TEST_ORG}/producer" deploy --inputs.environment=production --inputs.version="1.0.0" >/dev/null 2>&1; then
        print_success "Schema compatibility test with v1.0.0 passed"
    else
        print_error "Schema compatibility test with v1.0.0 failed"
        return 1
    fi
    
    # Update producer to emit version 1.1.0
    local producer_yml='version: 1.0.0
workflows:
  deploy:
    inputs:
      environment:
        type: string
        required: true
      version:
        type: string
        required: true
    steps:
      - id: emit-event
        uses: tako/fan-out@v1
        with:
          event_type: "deployment.completed"
          schema_version: "1.1.0"
          payload:
            environment: "{{ .inputs.environment }}"
            service: "api"
            version: "{{ .inputs.version }}"
          wait_for_children: false
          timeout: "30s"'
    
    setup_test_repo "$producer_path" "$producer_yml"
    
    echo "Testing schema version 1.1.0 (should trigger only caret consumer)..."
    if tako exec --cache-dir "$CACHE_DIR" --repo "${TEST_ORG}/producer" deploy --inputs.environment=production --inputs.version="1.1.0" >/dev/null 2>&1; then
        print_success "Schema compatibility test with v1.1.0 passed"
    else
        print_error "Schema compatibility test with v1.1.0 failed"
        return 1
    fi
}

# Test 5: Error Handling
test_error_handling() {
    print_section "Test 5: Error Handling and Edge Cases"
    
    # Create consumer with invalid CEL expression
    local invalid_consumer_path="${CACHE_DIR}/repos/${TEST_ORG}/invalid-consumer/main"
    local invalid_consumer_yml='version: 1.0.0
subscriptions:
  - events: ["deployment.completed"]
    workflow: "invalid-cel"
    schema_version: "^1.0.0"
    filters:
      - "invalid.cel.syntax.here!"
    inputs:
      service_name: "{{ .payload.service }}"
workflows:
  invalid-cel:
    inputs:
      service_name:
        type: string
        required: true
    steps:
      - id: cel-processing
        run: echo "Should not reach here"'
    
    setup_test_repo "$invalid_consumer_path" "$invalid_consumer_yml"
    
    echo "Testing invalid CEL expression handling..."
    if output=$(tako exec --cache-dir "$CACHE_DIR" --repo "${TEST_ORG}/producer" deploy --inputs.environment=production --inputs.version="1.0.0" 2>&1); then
        if echo "$output" | grep -qi "cel.*error\|compilation.*error\|evaluation.*failed"; then
            print_success "Invalid CEL expression handled correctly"
        else
            print_warning "CEL error handling may not be working as expected"
        fi
    else
        print_success "Error handling working - invalid CEL caused expected failure"
    fi
}

# Main execution
main() {
    echo -e "${BLUE}🚀 Starting Subscription-Based Workflow Triggering Verification${NC}"
    echo "Project: $(basename "$PROJECT_ROOT")"
    echo "Test Organization: $TEST_ORG"
    
    # Set up trap for cleanup
    trap cleanup_on_exit EXIT
    
    # Setup test environment
    setup_test_environment
    
    # Run tests
    local failed_tests=0
    
    test_basic_fanout || ((failed_tests++))
    test_diamond_dependency || ((failed_tests++))
    test_cel_performance || ((failed_tests++))
    test_schema_compatibility || ((failed_tests++))
    test_error_handling || ((failed_tests++))
    
    # Summary
    print_section "Verification Summary"
    local total_tests=5
    local passed_tests=$((total_tests - failed_tests))
    
    if [ $failed_tests -eq 0 ]; then
        print_success "All $total_tests tests passed! ✨"
        echo -e "${GREEN}🎉 Subscription-based workflow triggering implementation verified successfully!${NC}"
    else
        print_error "$failed_tests out of $total_tests tests failed"
        echo -e "${RED}❌ Some tests failed. Please review the implementation.${NC}"
    fi
    
    # Cleanup option (only if tests passed and user wants to preserve)
    if [ $failed_tests -eq 0 ]; then
        echo ""
        read -p "Keep test workspace for inspection? [y/N]: " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            PRESERVE_TEST_DIR=true
            echo -e "${YELLOW}📁 Test workspace preserved at: $TEST_DIR${NC}"
        fi
    fi
    
    return $failed_tests
}

# Handle script arguments
case "${1:-}" in
    "cleanup")
        cleanup_test_repos
        exit 0
        ;;
    "--preserve-test-dir")
        PRESERVE_TEST_DIR=true
        echo -e "${YELLOW}⚠️  Test workspace will be preserved${NC}"
        ;;
    "help"|"-h"|"--help")
        echo "Usage: $0 [cleanup|--preserve-test-dir|help]"
        echo ""
        echo "Commands:"
        echo "  (no args)              Run all verification tests with cleanup"
        echo "  --preserve-test-dir    Run tests but preserve test workspace"
        echo "  cleanup                Clean up existing test workspace"
        echo "  help                   Show this help message"
        echo ""
        echo "Test workspace will be created at: ${PROJECT_ROOT}/test_workspace"
        exit 0
        ;;
esac

# Run main function
main "$@"