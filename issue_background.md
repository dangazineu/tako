# Issue #106 Background: Subscription-Based Workflow Triggering

## Issue Summary
Implement the subscription-based workflow triggering system that evaluates event filters and maps events to workflows in child repositories.

## Key Requirements
- Lazy evaluation for repositories in dependency tree only
- At-least-once delivery with idempotency handling
- Diamond dependency resolution (first-subscription-wins)
- Schema compatibility validation
- CEL filter evaluation with performance optimizations

## Related Work and Dependencies

### Parent Epic: #21 - Execute multi-step workflows
- Foundational requirement for multi-repository workflow automation
- Established the vision for dependency-aware execution

### Design Issue: #98 - Event-driven workflow engine design
- **Status**: CLOSED ✅
- Comprehensive design for the general-purpose workflow engine
- Established key requirements including:
  - Event-driven multi-repository orchestration
  - First-class artifacts and state management
  - Containerized execution environments
  - Graph-aware workflow execution

### Dependency: #105 - tako/fan-out@v1 semantic step 
- **Status**: CLOSED ✅
- Implemented the core fan-out step for event emission
- Created the foundation for event-driven architecture
- Provides the `FanOutExecutor` that will trigger subscriptions

## Current Implementation Status

### ✅ Already Implemented
Based on code analysis, the following components are already in place:

1. **Subscription Configuration (`internal/config/subscription.go`)**
   - `Subscription` struct with all required fields
   - Validation for artifact references, event types, schema versions
   - Support for CEL filters and input mappings

2. **Event Model (`internal/engine/event_model.go`)**
   - Comprehensive event validation and schema management
   - Event serialization/deserialization
   - Schema compatibility checking

3. **Subscription Evaluation (`internal/engine/subscription.go`)**
   - `SubscriptionEvaluator` with CEL environment setup
   - Event-subscription matching with filters
   - Schema version compatibility checking
   - Simple template processing for input mapping

4. **Repository Discovery (`internal/engine/discovery.go`)**
   - `DiscoveryManager` for finding subscriber repositories
   - Cache-based repository scanning
   - Subscription loading from `tako.yml` files

5. **Fan-Out Execution (`internal/engine/fanout.go`)**
   - `FanOutExecutor` framework is in place
   - State management and monitoring components
   - Circuit breaker and retry logic

### ❌ Missing Implementation
The issue description mentions creating new files, but analysis shows these already exist. What appears to be missing is the **integration** between components:

1. **Subscription Processing Integration**
   - Connection between fan-out step and subscription evaluation
   - Workflow triggering based on subscription matches
   - Idempotency checking implementation

2. **Diamond Dependency Resolution**
   - First-subscription-wins policy implementation
   - Conflict detection and resolution

3. **Performance Optimizations**
   - CEL filter evaluation caching
   - Lazy repository loading

## Configuration Schema
The subscription configuration is already supported in `tako.yml`:

```yaml
subscriptions:
  - artifact: my-org/go-lib:go-lib
    events: [library_built]
    schema_version: "^1.0.0"
    filters:
      - semver.major(event.payload.version) > 0
      - has(event.payload.commit_sha)
    workflow: update_integration
    inputs:
      version: "{{ .event.payload.version }}"
```

## Architecture Overview
The system follows this flow:
1. Fan-out step emits event with payload
2. Discovery manager finds repositories with matching subscriptions
3. Subscription evaluator filters matches using CEL expressions
4. Schema compatibility is validated
5. Workflow is triggered in subscriber repositories with mapped inputs

## Testing Coverage
- Comprehensive unit tests exist for all core components
- Event model validation is well-tested
- Subscription evaluation has extensive test coverage
- Discovery manager has basic test coverage

## Next Steps
The main work appears to be **connecting the existing components** rather than creating new ones from scratch. The files mentioned in the issue (`subscriptions.go`, `schema.go`, `idempotency.go`) may be organizational refactoring rather than net-new functionality.