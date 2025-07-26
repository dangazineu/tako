# Testing Plan: Tako Exec Workflow Engine

This document outlines the comprehensive testing strategy for the Tako exec workflow engine, covering unit tests, integration tests, end-to-end tests, and performance validation. The testing plan is aligned with the event-driven architecture using `tako/fan-out@v1` steps and subscription-based orchestration.

## 1. Testing Strategy Overview

### 1.1. Testing Pyramid

```
                    ┌─────────────────┐
                    │  E2E Tests      │  Multi-repo scenarios
                    │  (Complex)      │  Event propagation
                    └─────────────────┘  Long-running workflows
                  ┌─────────────────────┐
                  │  Integration Tests  │  Component interaction
                  │  (Moderate)         │  Built-in steps
                  └─────────────────────┘  Container execution
              ┌─────────────────────────────┐
              │  Unit Tests                 │  Individual functions
              │  (High Volume)              │  State management
              └─────────────────────────────┘  Template processing
```

### 1.2. Testing Categories

1. **Unit Tests**: Individual component validation
2. **Integration Tests**: Cross-component functionality  
3. **End-to-End Tests**: Complete workflow scenarios
4. **Performance Tests**: Scalability and resource usage
5. **Security Tests**: Container isolation and secret handling
6. **Chaos Tests**: Failure scenarios and recovery

## 2. Unit Testing

### 2.1. Configuration Schema Testing

**Location**: `internal/config/`

**Test Coverage**:
- Schema validation for all workflow configurations
- Input validation (type conversion, enum validation, regex patterns)
- Subscription criteria parsing and validation
- Event schema validation with version compatibility
- Error message quality for invalid configurations

**Key Test Cases**:
```go
func TestWorkflowSchemaValidation(t *testing.T) {
    // Valid workflow configurations
    // Invalid input types and validation rules
    // Subscription syntax validation (repo:artifact format)
    // Event schema version compatibility
}

func TestSubscriptionParsing(t *testing.T) {
    // Valid subscription criteria
    // CEL filter expression validation
    // Event type matching
    // Schema version range validation
}
```

### 2.2. Template Engine Testing

**Location**: `internal/engine/template.go`

**Test Coverage**:
- Template variable resolution (`.inputs`, `.steps`, `.trigger`)
- Security functions (`shell_quote`, `json_escape`, etc.)
- Custom template functions for iteration
- Template caching performance
- Error handling for malformed templates

**Key Test Cases**:
```go
func TestTemplateResolution(t *testing.T) {
    // Basic variable substitution
    // Complex nested contexts
    // Shell escaping and security functions
    // Iteration over trigger artifacts
    // Template cache hit/miss scenarios
}
```

### 2.3. State Management Testing

**Location**: `internal/engine/state.go`

**Test Coverage**:
- State persistence and loading
- Checksum validation and corruption detection
- Backup creation and recovery
- State size limits and warnings
- Concurrent access handling

**Key Test Cases**:
```go
func TestStatePersistence(t *testing.T) {
    // Basic state save/load cycles
    // Corruption detection and recovery
    // State file size limits
    // Backup fallback scenarios
}
```

### 2.4. CEL Expression Testing

**Location**: `internal/engine/cel.go`

**Test Coverage**:
- Expression evaluation with different contexts
- Security sandboxing and resource limits
- Custom function library (semver, artifact functions)
- Error handling for invalid expressions
- Performance testing for complex expressions

## 3. Integration Testing

### 3.1. Built-in Steps Testing

**Location**: `internal/steps/`

**Test Coverage**:
- `tako/checkout@v1` with different ref parameters
- `tako/update-dependency@v1` with multiple ecosystems
- `tako/create-pull-request@v1` with GitHub API integration
- `tako/fan-out@v1` event emission and propagation
- `tako/poll@v1` for long-running step monitoring

**Key Test Cases**:
```go
func TestFanOutStep(t *testing.T) {
    // Event emission with correct payload
    // Multiple repository triggering
    // Timeout and failure handling
    // Deep synchronization (DFS traversal)
}

func TestUpdateDependencyStep(t *testing.T) {
    // Go module updates
    // NPM package updates  
    // Maven dependency updates
    // Error handling for unsupported ecosystems
}
```

### 3.2. Container Execution Testing

**Location**: `internal/engine/container.go`

**Test Coverage**:
- Security hardening (non-root, read-only filesystem)
- Resource limit enforcement
- Network isolation and selective access
- Container lifecycle management
- Image pull policies and private registries

**Key Test Cases**:
```go
func TestContainerSecurity(t *testing.T) {
    // Non-root execution verification
    // Capability dropping validation
    // Network isolation testing
    // Filesystem access restrictions
}

func TestResourceLimits(t *testing.T) {
    // CPU limit enforcement
    // Memory limit enforcement  
    // Resource exhaustion handling
    // Hierarchical limit inheritance
}
```

### 3.3. Event System Testing

**Location**: `internal/engine/events.go`

**Test Coverage**:
- Event emission and namespacing
- Subscription matching and filtering
- At-least-once delivery semantics
- Schema evolution and compatibility
- Diamond dependency resolution

**Key Test Cases**:
```go
func TestEventDelivery(t *testing.T) {
    // Event namespacing (repo/event format)
    // Subscription filter evaluation
    // Idempotency in child repositories
    // Schema version compatibility
}

func TestDiamondDependencies(t *testing.T) {
    // Multiple parents triggering same child
    // First-subscription-wins behavior
    // Event aggregation scenarios
}
```

## 4. End-to-End Testing

### 4.1. Single Repository Scenarios

**Scenario 1: Basic Workflow Execution**
```yaml
# Test workflow with inputs, outputs, and multiple steps
workflows:
  test-workflow:
    inputs:
      environment:
        type: string
        validation:
          enum: [dev, staging, prod]
    steps:
      - id: validate_input
        run: echo "Deploying to {{ .inputs.environment }}"
      - id: process_output
        run: echo "processed-{{ .steps.validate_input.outputs.result }}"
        produces:
          outputs:
            final_result: from_stdout
```

**Scenario 2: Long-Running Workflow with Resume**
```yaml
workflows:
  long-running-test:
    steps:
      - id: prepare
        run: ./scripts/prepare.sh
      - id: long_process
        long_running: true
        run: ./scripts/long-process.sh
      - id: poll_completion
        uses: tako/poll@v1
        with:
          target: step
          step_id: long_process
          timeout: 30m
      - id: finalize
        run: ./scripts/finalize.sh
```

### 4.2. Multi-Repository Event-Driven Scenarios

**Scenario 3: Fan-Out/Fan-In with Event-Driven Architecture**

This scenario tests the new event-driven model with explicit fan-out steps.

**Parent Repository** (`go-lib/tako.yml`):
```yaml
version: 0.1.0
artifacts:
  go-lib:
    path: ./go.mod
    ecosystem: go

workflows:
  release:
    inputs:
      version-bump:
        type: string
        default: "patch"
        validation:
          enum: [major, minor, patch]
    steps:
      - id: build_library
        run: ./scripts/build.sh --bump {{ .inputs.version-bump }}
        produces:
          artifact: go-lib
          outputs:
            version: from_stdout
          events:
            - type: library_built
              payload:
                version: "{{ .outputs.version }}"
                commit_sha: "{{ .env.GITHUB_SHA }}"
                
      - id: fan_out_to_dependents
        uses: tako/fan-out@v1
        with:
          event_type: library_built
          wait_for_children: true
          timeout: "1h"
          
      - id: create_final_release
        run: ./scripts/create-release.sh --version {{ .steps.build_library.outputs.version }}
```

**Child Repository A** (`app-one/tako.yml`):
```yaml
version: 0.1.0
artifacts:
  app-one-service:
    path: ./package.json
    ecosystem: npm

subscriptions:
  - artifact: my-org/go-lib:go-lib
    events: [library_built]
    filters:
      - semver.major(event.payload.version) > 0 || semver.minor(event.payload.version) > 0
    workflow: update_integration
    inputs:
      upstream_version: "{{ .event.payload.version }}"

workflows:
  update_integration:
    inputs:
      upstream_version:
        type: string
        required: true
    steps:
      - uses: tako/checkout@v1
      - uses: tako/update-dependency@v1
        with:
          name: go-lib
          version: "{{ .inputs.upstream_version }}"
      - id: run_tests
        run: npm test
      - id: build_service
        run: npm run build
        produces:
          artifact: app-one-service
          outputs:
            version: from_stdout
          events:
            - type: service_updated
              payload:
                version: "{{ .outputs.version }}"
                upstream_version: "{{ .inputs.upstream_version }}"
      - id: fan_out_to_dependents
        uses: tako/fan-out@v1
        with:
          event_type: service_updated
          wait_for_children: true
```

**Child Repository B** (`app-two/tako.yml`):
```yaml
version: 0.1.0
artifacts:
  app-two-service:
    path: ./pom.xml
    ecosystem: maven

subscriptions:
  - artifact: my-org/go-lib:go-lib
    events: [library_built]
    workflow: update_integration
    inputs:
      upstream_version: "{{ .event.payload.version }}"

workflows:
  update_integration:
    inputs:
      upstream_version:
        type: string
        required: true
    steps:
      - uses: tako/checkout@v1
      - uses: tako/update-dependency@v1
        with:
          name: go-lib
          version: "{{ .inputs.upstream_version }}"
      - id: run_tests
        run: mvn test
      - id: build_service
        run: mvn package
        produces:
          artifact: app-two-service
          outputs:
            version: from_stdout
          events:
            - type: service_updated
              payload:
                version: "{{ .outputs.version }}"
                upstream_version: "{{ .inputs.upstream_version }}"
```

**Aggregation Repository** (`release-bom/tako.yml`):
```yaml
version: 0.1.0

subscriptions:
  - artifact: my-org/app-one:app-one-service
    events: [service_updated]
    workflow: update_bom
    inputs:
      service_name: "app-one"
      service_version: "{{ .event.payload.version }}"
  - artifact: my-org/app-two:app-two-service  
    events: [service_updated]
    workflow: update_bom
    inputs:
      service_name: "app-two"
      service_version: "{{ .event.payload.version }}"

workflows:
  update_bom:
    inputs:
      service_name:
        type: string
        required: true
      service_version:
        type: string
        required: true
    steps:
      - uses: tako/checkout@v1
      - id: update_bom_file
        run: ./scripts/update-bom.sh --service {{ .inputs.service_name }} --version {{ .inputs.service_version }}
      - id: commit_changes
        run: git add . && git commit -m "Update {{ .inputs.service_name }} to {{ .inputs.service_version }}"
```

**Test Validation**:
1. Execute `tako exec release --repo=my-org/go-lib --inputs.version-bump=minor`
2. Verify library builds and emits `library_built` event
3. Confirm both `app-one` and `app-two` workflows are triggered in parallel
4. Validate subscription filter evaluation (major/minor vs patch)
5. Ensure each app emits its own `service_updated` event
6. Verify `release-bom` receives both events and updates accordingly
7. Confirm parent waits for all children before proceeding to final release step

### 4.3. Complex Multi-Repository Scenarios

**Scenario 4: Cascading Updates with Multiple Event Types**

Tests multiple levels of dependencies with different event types and complex aggregation.

**Scenario 5: Partial Failure Recovery**

Tests smart partial resume functionality:
1. Trigger multi-repository workflow
2. Simulate failure in one child repository
3. Verify other repositories complete successfully
4. Resume only failed branch with `tako exec --resume <run-id> --failed-only`
5. Validate successful completion of entire execution tree

**Scenario 6: Long-Running Multi-Repository Workflow**

Tests asynchronous execution across repositories:
1. Parent triggers children with long-running steps
2. Verify parent can exit while children continue
3. Test status monitoring across repositories
4. Resume workflow after all long-running steps complete

**Scenario 7: Thundering Herd on Resume**

Tests system resilience when resuming workflows with many children:
1. Create workflow with ~20 child repositories
2. Pause all children at the same step (simulate network failure)
3. Resume parent workflow simultaneously
4. Monitor resource utilization (CPU, network, API rate limits)
5. Verify `--max-concurrent-repos` prevents cascading failures
6. Validate graceful handling of sudden resource demand

**Scenario 8: Workflow Cancellation and Cleanup**

Tests explicit workflow cancellation across repositories:
1. Start multi-repository workflow with long-running steps
2. Issue `tako cancel <run-id>` command while in progress
3. Verify in-flight steps are gracefully terminated
4. Ensure no new steps are started after cancellation
5. Validate cleanup actions are executed across all repositories
6. Test partial cancellation (some repos complete, others are cancelled)

**Scenario 9: Empty Fan-Out and Edge Cases**

Tests edge cases in event propagation:
1. **Empty Fan-Out**: `tako/fan-out@v1` emits event with no subscribers
2. **Self-Subscription**: Repository subscribes to its own events
3. **Multiple Subscription Matches**: Multiple subscriptions in same repo match same event
4. Verify no indefinite waiting or unexpected errors
5. Validate clear behavior documentation for each case

## 5. Performance Testing

### 5.1. Scalability Tests

**Dependency Graph Scaling**:
- Test with 10, 25, 50, and 75 repositories
- Measure discovery phase performance
- Validate warning at 50 repositories threshold
- Test memory usage with large dependency trees

**Execution Tree Depth**:
- Test deep, narrow dependency chains (A → B → C → ... → J)
- Measure end-to-end execution time with depth
- Validate DFS traversal performance
- Test warning and failure thresholds for tree depth (7/10 levels)
- Monitor state propagation delay up the chain

**Concurrent Execution**:
- Test `--max-concurrent-repos` limits (1, 4, 8, 16)
- Measure resource utilization at different concurrency levels
- Validate repository-level locking behavior
- Test deadlock detection with complex dependency chains

**State Polling Performance**:
- Monitor network traffic and CPU usage for state polling
- Measure delay between child completion and parent recognition
- Validate exponential backoff strategy effectiveness (30s → 5m)
- Test polling overhead with 75-repository scenarios

**State Management Performance**:
- Test state file growth with large workflows
- Validate 10MB warning and 100MB failure thresholds
- Measure state persistence performance
- Test backup creation and recovery performance

### 5.2. Resource Limits Testing

**Hierarchical Resource Management**:
- Test global → repository → step limit inheritance
- Validate resource exhaustion prevention
- Test resource limit warnings (90% threshold)
- Measure resource monitoring overhead

**Template Cache Performance**:
- Test LRU eviction at 100MB limit
- Measure template parsing performance
- Validate cache hit/miss ratios
- Test memory usage with complex templates

## 6. Security Testing

### 6.1. Container Security Validation

**Security Hardening**:
- Verify non-root execution (UID 1001)
- Test read-only filesystem restrictions
- Validate capability dropping
- Test seccomp profile enforcement
- Verify network isolation

**Secret Management**:
- Test secret environment variable injection
- Validate secret scrubbing from logs
- Test template engine secret isolation
- Verify debug mode secret redaction

**Malicious Configuration Testing**:
- Test CEL expressions designed for denial-of-service attacks
- Validate protection against computationally expensive operations
- Test template injection attempts through secret leakage
- Validate `cache_key_files` pattern resource consumption limits
- Test malicious glob patterns that could cause performance issues

**Dependency Confusion Attack Prevention**:
- Test `tako/update-dependency@v1` with internal vs public package confusion
- Validate correct dependency resolution with custom registries
- Test package name collision scenarios
- Verify registry precedence and authentication

### 6.2. CEL Expression Security

**Sandboxing Validation**:
- Test 100ms timeout enforcement
- Validate 64MB memory limit
- Test filesystem access restrictions
- Verify network access prevention

## 7. Chaos and Failure Testing

### 7.1. System Resilience

**Network Partition Scenarios**:
- Test polling resilience during network issues
- Validate exponential backoff behavior
- Test timeout handling for aggregation scenarios

**Container Runtime Failures**:
- Test Docker daemon restart scenarios
- Validate container recovery after system reboot
- Test orphaned container cleanup (24-hour policy)

**State Corruption Scenarios**:
- Test state file corruption detection
- Validate automatic backup recovery
- Test partial state corruption handling

### 7.2. Resource Exhaustion

**Disk Space Exhaustion**:
- Test workspace cleanup behavior
- Validate cache management under disk pressure
- Test workflow behavior when disk space is low

**Memory Exhaustion**:
- Test behavior when hitting resource limits
- Validate graceful degradation
- Test OOM handling for long-running containers

## 8. Test Automation and CI Integration

### 8.1. Test Categories by Environment

**Local Development**:
- Unit tests: `go test -v -test.short ./...`
- Integration tests: `go test -v ./...`
- Local E2E tests: `go test -v -tags=e2e --local ./...`

**CI Environment**:
- All local tests plus remote E2E: `go test -v -tags=e2e --remote ./...`
- Performance tests: `go test -v -tags=performance ./...`
- Security tests: `go test -v -tags=security ./...`

**Nightly Builds**:
- Chaos tests: `go test -v -tags=chaos ./...`
- Scalability tests: `go test -v -tags=scalability ./...`
- Full integration suite with external dependencies

### 8.2. Test Data Management

**Repository Fixtures**:
- Standardized test repository structures
- Reproducible test scenarios
- Version-controlled test configurations
- Automated test data generation

**Cleanup Strategies**:
- Automatic workspace cleanup after tests
- Container cleanup for integration tests
- State file cleanup for failed tests
- Test repository cleanup in CI

## 9. Testing Tools and Infrastructure

### 9.1. Test Utilities

**Mock Services**:
- Mock GitHub API for pull request testing
- Mock container registry for image pull testing
- Mock event delivery for subscription testing

**Test Helpers**:
- Repository fixture generation
- Workflow state inspection utilities
- Event payload validation helpers
- Performance measurement utilities

### 9.2. Continuous Testing

**Test Monitoring**:
- Test execution time tracking
- Flaky test identification
- Performance regression detection
- Coverage reporting and trends

**Quality Gates**:
- Minimum 85% code coverage for new code
- All E2E scenarios must pass
- Performance tests must not regress by >10%
- Security tests must all pass

## 10. Advanced Testing Strategies

### 10.1. Property-Based Testing

**Template Engine Properties**:
```go
func TestTemplateSecurityProperty(t *testing.T) {
    // Property: For any valid workflow input and template string,
    // the resolved template should never contain raw secret values
    quick.Check(func(input WorkflowInput, template string) bool {
        resolved := resolveTemplate(template, input)
        return !containsSecretValues(resolved)
    }, nil)
}
```

**CEL Expression Properties**:
- Resource consumption should be bounded for any valid expression
- Evaluation should never access filesystem or network
- Timeout should be enforced regardless of expression complexity

### 10.2. Event-Driven System Testing Patterns

**Consumer-Driven Contract Testing**:
- Implement contract testing using Pact framework
- Child repositories define contracts for expected event schemas
- Parent repository CI validates event production against contracts
- Prevents breaking changes in event schemas

**Idempotency Testing**:
```yaml
# Test case: Deliver same event twice
workflows:
  idempotency_test:
    steps:
      - id: process_event
        run: ./scripts/idempotent-process.sh
        # Must produce identical results on repeated execution
```

**Event Ordering and Race Condition Testing**:
- Test rapid-fire event delivery scenarios
- Validate event processing order guarantees
- Test concurrent event processing from multiple sources

### 10.3. Mutation Testing

**Implementation Strategy**:
- Use Go-mutesting framework in nightly builds
- Target critical packages: `engine`, `config`, `state`
- Achieve >80% mutation score for core functionality
- Identify test gaps through mutation survivors

### 10.4. State Migration Testing

**Schema Evolution Scenarios**:
- Test state file backward compatibility across Tako versions
- Validate graceful handling of unknown state fields
- Test resume after binary upgrade scenarios
- Verify state corruption detection across versions

**Test Cases**:
```go
func TestStateMigration(t *testing.T) {
    // 1. Create workflow with old Tako version
    // 2. Pause workflow with long-running step
    // 3. Upgrade Tako binary 
    // 4. Resume workflow with new version
    // 5. Verify successful completion
}
```

This comprehensive testing plan ensures the Tako exec workflow engine is robust, performant, and secure across all supported scenarios and use cases. The additional advanced testing strategies provide confidence in system reliability, especially for the complex event-driven multi-repository orchestration scenarios.