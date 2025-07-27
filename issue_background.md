# Issue #105 Background: Implement tako/fan-out@v1 Semantic Step

**Date:** 2025-01-27
**Issue:** #105 - feat(engine): Implement tako/fan-out@v1 semantic step
**Milestone:** Milestone 4: Event-Driven Multi-Repository Orchestration

## Issue Overview

This issue implements the cornerstone of the event-driven architecture for Tako: the `tako/fan-out@v1` built-in step that enables workflows in parent repositories to trigger workflows in child repositories through structured events.

### Key Functionality to Implement

1. **Event Emission:** Emit events with schema versioning and structured payloads
2. **Repository Discovery:** Find all repositories subscribed to emitted events
3. **Subscription Evaluation:** Filter events using CEL expressions 
4. **Deep Synchronization:** DFS traversal with wait_for_children support
5. **Timeout Handling:** Prevent indefinite waiting in aggregation scenarios
6. **Parallel Execution:** Control concurrency with configurable limits

### Fan-Out Step Parameters

```yaml
- uses: tako/fan-out@v1
  with:
    event_type: library_built           # Required: event type to emit
    wait_for_children: true             # Optional: wait for all triggered workflows  
    timeout: "2h"                       # Optional: timeout for waiting
    concurrency_limit: 4                # Optional: max concurrent child executions
```

## Dependency Analysis

### Parent Epic: Issue #21 - Execute multi-step workflows
- **Status:** OPEN
- **Milestone:** Milestone 7: Workflows & Local Testing (Legacy)
- **Description:** Core workflow execution functionality 
- **Context:** Foundation for all workflow features, defines basic execution model

### Design Document: Issue #98 - Event-driven workflow engine design  
- **Status:** CLOSED
- **Description:** Comprehensive design for general-purpose workflow engine
- **Key Requirements:**
  - Workflow inputs and outputs
  - First-class artifacts
  - Graph-aware execution
  - Containerized execution
  - State persistence and resumption
  - Secrets management

### Implementation Dependencies

#### Issue #101 - Core execution engine (CLOSED)
- **Key Components Implemented:**
  - `internal/engine/runner.go` - execution orchestration
  - `internal/engine/state.go` - execution tree state management  
  - `internal/engine/workspace.go` - copy-on-write isolation
  - `internal/engine/locks.go` - fine-grained locking with deadlock detection
- **Features Available:**
  - Timestamp-based run ID generation
  - Multi-repository execution trees
  - Smart partial resume capability
  - Repository-level locking

#### Issue #102 - Template engine with event context (CLOSED)
- **Key Components Implemented:**
  - `internal/engine/template.go` - template processing with event context
  - `internal/engine/security.go` - security functions (shell_quote, etc.)
  - `internal/engine/context.go` - template context management
- **Features Available:**
  - Event context (`.event.payload`) support
  - Security functions for safe template processing
  - Template caching with LRU eviction
  - Custom functions for event payload processing

## Current Architecture State

### Configuration Schema (Already Implemented)

**Event Structure:** (`internal/config/events.go`)
```go
type Event struct {
    Type          string            `yaml:"type"`
    SchemaVersion string            `yaml:"schema_version,omitempty"`  
    Payload       map[string]string `yaml:"payload,omitempty"`
}
```

**Subscription Structure:** (`internal/config/subscription.go`)
```go
type Subscription struct {
    Artifact      string            `yaml:"artifact"`                 // Format: repo:artifact
    Events        []string          `yaml:"events"`                   // Event types to subscribe to
    SchemaVersion string            `yaml:"schema_version,omitempty"` // Version range
    Filters       []string          `yaml:"filters,omitempty"`        // CEL expressions
    Workflow      string            `yaml:"workflow"`                 // Workflow to trigger
    Inputs        map[string]string `yaml:"inputs,omitempty"`         // Input mappings
}
```

**Workflow Step Structure:** (`internal/config/config.go`)
```go
type WorkflowStep struct {
    ID              string                 `yaml:"id,omitempty"`
    Uses            string                 `yaml:"uses,omitempty"`          // Built-in step reference
    With            map[string]interface{} `yaml:"with,omitempty"`          // Step parameters
    // ... other fields for run, image, etc.
}
```

### Engine Infrastructure (Already Implemented)

**Runner Framework:** (`internal/engine/runner.go`)
- Built-in step infrastructure with `executeBuiltinStep()` method
- Currently returns "not yet implemented" error for all built-in steps
- Integration with execution state, workspace management, and container management
- Template processing capabilities for step parameters

**State Management:** (`internal/engine/state.go`)
- Execution tree persistence across repositories
- Step completion tracking and resume capabilities
- Lock acquisition state management

**Container Integration:** (`internal/engine/container.go`)
- Docker runtime detection and management
- Resource constraints and security profiles
- Network isolation capabilities

### Built-in Step Integration Point

Current implementation in `runner.go` (line ~580):
```go
// Check if this is a built-in step (uses: field)
if step.Uses != "" {
    return r.executeBuiltinStep(step, stepID, startTime)
}
```

The `executeBuiltinStep` method currently returns:
```go
err := fmt.Errorf("built-in steps not yet implemented: %s", step.Uses)
```

## Implementation Scope

### New Files to Create

1. **`internal/steps/fanout.go`** - Core fan-out step implementation
2. **`internal/engine/discovery.go`** - Repository discovery and subscription lookup
3. **`internal/engine/subscription.go`** - Subscription evaluation and filtering

### Key Integration Points

1. **Built-in Step Registration:** Extend `executeBuiltinStep()` to handle `tako/fan-out@v1`
2. **Event Emission:** Create event payloads and emit to subscribed repositories  
3. **Subscription Processing:** Evaluate CEL filters and trigger target workflows
4. **Synchronization Logic:** Implement wait_for_children with timeout handling
5. **Concurrency Control:** Respect concurrency_limit parameter for parallel execution

### Existing Dependencies Available

- **Template Engine:** Event payload templating with security functions
- **Container Manager:** For containerized step execution if needed
- **State Manager:** For tracking fan-out execution progress and resume
- **Lock Manager:** For coordinating concurrent repository access
- **Workspace Manager:** For isolated repository operations

## Technical Considerations

### Repository Discovery Strategy
- Need to implement repository scanning for subscription matching
- Must support artifact-based subscription filtering (`repo:artifact` format)
- Should integrate with existing caching infrastructure

### Event Schema Versioning
- Support semantic version ranges in subscriptions
- Validate event schema compatibility before triggering workflows
- Handle version mismatches gracefully

### Deep Synchronization (DFS)
- Implement depth-first traversal for complete execution trees
- Track nested fan-out operations across multiple repository levels
- Aggregate results from entire execution subtree

### Error Handling and Timeouts
- Implement configurable timeout support (default: no timeout)
- Handle partial failures in fan-out scenarios
- Provide detailed failure reporting for debugging

### Performance and Scalability
- Respect concurrency limits to prevent resource exhaustion
- Use existing workspace isolation for concurrent operations
- Leverage repository caching to minimize clone overhead

## Success Criteria

1. **Event Emission:** Events emitted with correct schema versioning and payload
2. **Repository Discovery:** Find all repositories subscribed to specific events
3. **Subscription Filtering:** CEL expressions evaluated correctly for event filtering
4. **Deep Synchronization:** Complete execution tree waits properly with DFS traversal
5. **Timeout Handling:** Configurable timeouts prevent indefinite blocking
6. **Concurrency Control:** Parallel execution respects specified limits
7. **Integration:** Seamless integration with existing runner and state management
8. **Testing:** Comprehensive test coverage for all fan-out scenarios

## Related Issues and Context

- **Current Milestone:** Part of "Event-Driven Multi-Repository Orchestration"
- **Follow-up Work:** This implementation enables event-driven workflows across repositories
- **Testing Strategy:** Will require multi-repository test scenarios to validate end-to-end flow
- **Documentation:** Updates needed for built-in step documentation and examples