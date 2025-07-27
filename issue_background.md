# Issue #105 Background: Implement tako/fan-out@v1 semantic step

## Issue Overview
This issue implements the `tako/fan-out@v1` built-in step that enables event-driven multi-repository orchestration through explicit fan-out operations. This is the cornerstone issue of the event-driven architecture.

## Parent Epic & Dependencies
- **Parent Epic**: #21 - Execute multi-step workflows 
- **Design**: #98 - Event-driven workflow engine design (CLOSED)
- **Dependencies**: 
  - #101 - Core execution engine with state management (CLOSED)
  - #102 - Template engine with event context (CLOSED)

## Key Functionality Requirements
1. **Event emission with schema versioning**: Emit structured events with proper versioning
2. **Deep synchronization with DFS traversal**: Wait for complete execution tree to finish
3. **Timeout handling for aggregation scenarios**: Prevent indefinite waiting
4. **Parallel execution with concurrency limits**: Manage concurrent child executions

## Fan-Out Step Parameters
```yaml
- uses: tako/fan-out@v1
  with:
    event_type: library_built           # Required: event type to emit
    wait_for_children: true             # Optional: wait for all triggered workflows
    timeout: "2h"                       # Optional: timeout for waiting
    concurrency_limit: 4                # Optional: max concurrent child executions
```

## Current Implementation Status

### What's Already Implemented
1. **Core Config Support**: `tako/fan-out@v1` is already listed in `knownBuiltinSteps` in `/internal/config/config.go:261`
2. **Event System**: Complete event structure in `/internal/config/events.go`
3. **Subscription System**: Complete subscription system in `/internal/config/subscription.go`
4. **Execution Engine**: Full execution engine with state management and workspace isolation in `/internal/engine/runner.go`
5. **Template Engine**: Event context support with `.event.payload` available
6. **Container Management**: Containerized execution environment ready

### What's Missing (This Issue)
1. **Built-in Step Implementation**: The `executeBuiltinStep` function in `/internal/engine/runner.go:522` has a TODO comment and returns "not yet implemented"
2. **Repository Discovery**: Need to implement discovery of repositories with subscriptions
3. **Subscription Evaluation**: Need to evaluate subscription filters with CEL expressions
4. **Event Emission**: Need to actually emit events to trigger child workflows
5. **Deep Synchronization**: Need DFS traversal logic for waiting on complete execution tree

## File Structure Analysis

### Current Engine Structure
- `/internal/engine/runner.go` - Main execution engine (where built-in steps are called)
- `/internal/engine/state.go` - State management for execution trees
- `/internal/engine/template.go` - Template processing with event context
- `/internal/engine/locks.go` - Fine-grained repository locking
- `/internal/engine/workspace.go` - Workspace isolation

### Missing Files (Need to Create)
- `/internal/steps/fanout.go` - Central fan-out step implementation
- `/internal/engine/discovery.go` - Repository discovery logic
- `/internal/engine/subscription.go` - Subscription evaluation logic

## Architecture Integration Points

### Event-Driven Architecture
The system uses centralized orchestration with distributed subscriptions:
- Parent repositories use `tako/fan-out@v1` steps to emit events
- Child repositories declare subscriptions to events from other repositories
- Events are structured messages with schema versioning

### Execution Flow
1. Parent workflow reaches `tako/fan-out@v1` step
2. Step emits event with specified type and payload
3. Engine discovers repositories with matching subscriptions
4. Engine evaluates subscription filters (CEL expressions)
5. Engine triggers workflows in matching repositories
6. If `wait_for_children: true`, step waits for all triggered workflows to complete
7. Step completes when all child executions finish or timeout is reached

### State Management Integration
The execution engine already supports:
- Hierarchical state management for multi-repository workflows
- Timestamp-based run ID generation
- Copy-on-write workspace isolation
- Smart partial resume capability

## Testing Requirements
Based on AIRULES.md, need comprehensive testing:
1. Unit tests for all new functionality
2. Integration tests for event emission and subscription matching
3. End-to-end tests for complete fan-out scenarios
4. Error handling tests for timeout and failure scenarios

## Acceptance Criteria
- [ ] Events emitted with correct schema versioning and payload
- [ ] Repository discovery finds all subscribed repositories
- [ ] Subscription filter evaluation works with CEL expressions
- [ ] Deep synchronization waits for complete execution tree
- [ ] Timeout handling prevents indefinite waiting