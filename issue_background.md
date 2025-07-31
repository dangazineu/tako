# Issue #145 - Protobuf API Evolution E2E Test - Background Research

## Issue Requirements

The issue requires implementing a complex, real-world end-to-end testing scenario that verifies tako's advanced fan-out orchestration capabilities. Specifically:

### Scenario: Cross-Repository API Evolution with Protobuf
- **Goal**: When a new version of a shared API is released in a central repository, tako must automatically trigger specific downstream services affected by the change, while leaving others untouched.

### The Cast of Repositories
1. **`api-definitions` (Publisher)**: Central repository containing `.proto` files
2. **`go-user-service` (Consumer)**: Go microservice implementing UserService
3. **`nodejs-billing-service` (Consumer)**: Node.js microservice client to UserService  
4. **`go-legacy-service` (Non-Consumer)**: Older service that should be ignored

### Key Features to Test
- **Conditional triggering** based on CEL filter expressions
- **Selective fan-out** affecting only specified services
- **Event payload data** passed correctly as workflow inputs
- **Mock execution** with verifiable file-based side effects
- **Non-consumer isolation** ensuring unaffected services are left alone

## Related Previous Work and Dependencies

### 1. Fan-out System Foundation (PR #127, #128, #141)
- **PR #127**: Implemented `tako/fan-out@v1` semantic step with event-driven orchestration
- **PR #128**: Added subscription-based workflow triggering with idempotency
- **PR #141**: Implemented idempotency for fan-out state management

Key capabilities already available:
- Event emission with payload validation
- Repository discovery and subscription loading
- CEL filtering for conditional workflow triggering  
- Concurrent execution with limits
- State management and persistence
- Schema validation with semver ranges

### 2. E2E Testing Infrastructure
- Existing framework in `e2e_test.go` with TestCase/Step/Verification structure
- Template-based repository creation in `test/e2e/templates/`
- `takotest` utility for setup/cleanup operations
- Support for both local and remote testing modes
- Placeholder replacement system for dynamic test data

### 3. Existing Fan-out Test Template
- `test/e2e/templates/fan-out-test/` already exists with basic publisher/subscriber setup
- Shows event emission and subscription patterns
- Uses simple success-based filtering

## Integration Points with Existing Features

### 1. Event Model System  
- Located in `internal/engine/event_model.go`
- Supports schema validation, event building, serialization
- Already handles payload transformation and template resolution

### 2. Discovery Manager
- In `internal/engine/discovery.go`  
- Handles repository scanning and subscription loading
- Supports artifact reference resolution

### 3. Subscription Evaluation
- In `internal/engine/subscription.go`
- CEL expression evaluation with caching
- Semantic version matching
- Input mapping from event payload

### 4. Fan-out Execution
- In `internal/engine/fanout.go`
- Orchestrates complete fan-out lifecycle
- Handles concurrent child workflow execution
- State persistence and recovery

## Overall Project Architecture Context

### 1. Configuration Schema
- Tako.yml v1 format with events/subscriptions sections
- Support for event production and consumption declarations
- Template expressions for dynamic payload mapping
- CEL filter expressions for conditional triggering

### 2. Workflow Execution Model
- Child workflow execution via `ChildWorkflowExecutor`
- Workspace isolation and cleanup
- Local and remote execution modes
- Mock execution support for testing

### 3. Cache and State Management
- Repository caching in `~/.tako/cache/repos/`
- Fan-out state persistence for idempotency
- Lock management for concurrent operations

## Previous Attempts and Challenges

Based on the merged PRs, previous challenges that were resolved:
1. **Race conditions** in state persistence (fixed in PR #128)
2. **Thread safety** in CEL expression caching (addressed)
3. **Diamond dependency resolution** with first-subscription-wins policy
4. **Performance optimization** of CEL evaluation (30-60x improvement)
5. **Schema compatibility** and version range handling

No previous attempts at this specific protobuf scenario were found, making this a new test case.

## Key Implementation Considerations

### 1. Template Design  
Need to create 4 new repository templates under `test/e2e/templates/protobuf-api-evolution/`:
- `api-definitions/` with basic proto files and fanout workflow
- `go-user-service/` with subscription to specific service filter
- `nodejs-billing-service/` with similar subscription
- `go-legacy-service/` with no subscriptions (control group)

### 2. Test Case Structure
- Use existing E2E framework in `get_test_cases.go`  
- Add new environment in `environments.go`
- Create verification steps that check file existence
- Mock the actual protobuf compilation with simple file touches

### 3. CEL Filter Complexity
The scenario requires advanced CEL filtering:
```cel
'user-service' in event.payload.services_affected.split(',')
```

This tests:
- String manipulation functions in CEL
- Array operations and membership testing
- Complex payload data extraction

### 4. Mock Execution Strategy
Replace real deployment scripts with mock scripts that create verifiable side effects:
- `touch "go_user_service_deployed_with_api_$1"` instead of actual deployment
- File creation patterns that can be verified by test framework
- Predictable naming patterns for automation

## Risk Assessment

### Low Risk
- Using existing, well-tested fan-out infrastructure
- Following established E2E test patterns
- Mock execution eliminates external dependencies

### Medium Risk  
- Complex CEL expressions may reveal edge cases
- Multiple repository coordination in test setup
- Timing dependencies in concurrent workflow execution

### Mitigation Strategies
- Comprehensive unit tests for CEL expressions
- Use existing takotest infrastructure for reliable setup
- Add explicit synchronization in test verification steps