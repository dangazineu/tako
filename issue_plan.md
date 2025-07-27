# Implementation Plan: Issue #105 - tako/fan-out@v1 Semantic Step

**Date:** 2025-01-27
**Issue:** #105 - feat(engine): Implement tako/fan-out@v1 semantic step

## Overview

This plan implements the cornerstone `tako/fan-out@v1` built-in step for event-driven multi-repository orchestration. Based on comprehensive background analysis and technical consultation, the implementation follows a phased approach ensuring each phase leaves the codebase in a healthy, tested state.

## Technical Decisions

Based on consultation with Gemini and architecture analysis:

1. **Repository Discovery:** Separate `DiscoveryManager` following existing manager pattern
2. **Event Persistence:** Events stored in execution state for resumability
3. **Subscription Order:** Parallel execution with alphabetical sorting for determinism  
4. **Deep Synchronization:** Iterative DFS with cycle detection via execution state
5. **Error Handling:** Configurable fail-fast behavior (default: continue on failure)
6. **Schema Versioning:** Standard semver library with range support
7. **Concurrency:** Per fan-out step instance limits
8. **State Integration:** Hierarchical parent-child execution tree model

## Implementation Phases

### Phase 1: Core Infrastructure and Discovery
**Goal:** Establish repository discovery and subscription management foundation

#### 1.1 Discovery Manager Implementation
- **File:** `internal/engine/discovery.go`
- **Purpose:** Repository discovery and subscription lookup service
- **Key Functions:**
  - `NewDiscoveryManager(cacheDir string) *DiscoveryManager`
  - `FindSubscribers(artifact, eventType string) ([]SubscriptionMatch, error)`  
  - `LoadSubscriptions(repoPath string) ([]config.Subscription, error)`
- **Features:**
  - Repository scanning with caching integration
  - Artifact-based subscription filtering (`repo:artifact` format)
  - CEL expression evaluation for subscription filters
  - Integration with existing Git caching infrastructure

#### 1.2 Subscription Evaluation
- **File:** `internal/engine/subscription.go`
- **Purpose:** Event-subscription matching and filtering logic
- **Key Functions:**
  - `EvaluateSubscription(subscription config.Subscription, event Event) (bool, error)`
  - `CheckSchemaCompatibility(eventVersion, subscriptionRange string) (bool, error)`
  - `ProcessEventPayload(payload map[string]string, subscription config.Subscription) (map[string]string, error)`
- **Features:**
  - CEL expression evaluation for event filtering
  - Semantic version range checking using semver library
  - Template processing for input mappings
  - Event payload transformation and validation

#### 1.3 Basic Integration Tests
- **Purpose:** Verify discovery and subscription components work correctly
- **Test Coverage:**
  - Repository discovery with cached repositories
  - Subscription filtering with CEL expressions
  - Schema version compatibility checking
  - Error handling for malformed subscriptions

**Acceptance Criteria:**
- Discovery manager can find and load subscriptions from repositories
- Subscription evaluation correctly filters events using CEL expressions
- Schema version compatibility checking works with semver ranges
- All unit tests pass with >70% coverage maintained
- Integration tests verify component interaction

---

### Phase 2: Fan-Out Step Core Implementation  
**Goal:** Implement the basic fan-out step without deep synchronization

#### 2.1 Fan-Out Step Implementation
- **File:** `internal/steps/fanout.go`
- **Purpose:** Core fan-out step implementation
- **Key Functions:**
  - `ExecuteFanOut(step config.WorkflowStep, context *ExecutionContext) (StepResult, error)`
  - `EmitEvent(eventType string, payload map[string]string) error`
  - `TriggerSubscribers(subscribers []SubscriptionMatch, event Event) error`
- **Features:**
  - Event emission with schema versioning
  - Parallel subscriber triggering with concurrency limits
  - Basic error aggregation and reporting
  - Template processing for event payloads

#### 2.2 Runner Integration
- **File:** `internal/engine/runner.go` (modification)
- **Purpose:** Integrate fan-out step with built-in step execution
- **Changes:**
  - Extend `executeBuiltinStep()` to handle `tako/fan-out@v1`
  - Add fan-out step registration and parameter validation
  - Integrate with discovery manager and subscription services

#### 2.3 Event Model Enhancement
- **Files:** `internal/engine/events.go` (new)
- **Purpose:** Event structure and processing logic
- **Key Functions:**
  - `NewEvent(eventType, schemaVersion string, payload map[string]string) *Event`
  - `ValidateEvent(event *Event) error`
  - `SerializeEvent(event *Event) ([]byte, error)`
- **Features:**
  - Event validation and serialization
  - Payload template processing
  - Schema version management

#### 2.4 Basic Fan-Out Tests
- **Purpose:** Verify fan-out step executes correctly without deep sync
- **Test Coverage:**
  - Event emission with proper schema versioning
  - Subscriber discovery and triggering
  - Concurrency limit enforcement
  - Error handling for invalid parameters
  - Integration with existing runner infrastructure

**Acceptance Criteria:**
- Fan-out step can emit events and trigger immediate subscribers
- Concurrency limits are respected during parallel execution
- Event emission includes proper schema versioning and payload
- Integration with runner's built-in step infrastructure works correctly
- All tests pass with coverage maintained

---

### Phase 3: State Management and Deep Synchronization
**Goal:** Add hierarchical state tracking and wait_for_children support

#### 3.1 Hierarchical State Management
- **File:** `internal/engine/state.go` (modification)
- **Purpose:** Extend state manager for parent-child execution relationships
- **Changes:**
  - Add `CreateChildExecution(parentID, childRepo, workflow string) (string, error)`
  - Add `WaitForChildren(parentID string, timeout time.Duration) error`
  - Add `GetChildExecutionStatus(parentID string) ([]ChildStatus, error)`
- **Features:**
  - Parent-child execution tree modeling
  - Child execution status tracking
  - Hierarchical state persistence for resume
  - Timeout handling for child completion

#### 3.2 Deep Synchronization (DFS)
- **File:** `internal/steps/fanout.go` (enhancement)
- **Purpose:** Implement deep synchronization with DFS traversal
- **Features:**
  - Iterative DFS implementation with explicit stack
  - Cycle detection using execution state graph
  - `wait_for_children` parameter support
  - Timeout configuration with graceful degradation

#### 3.3 Resume Functionality Enhancement
- **File:** `internal/engine/state.go` (enhancement)
- **Purpose:** Support resume for interrupted fan-out operations
- **Features:**
  - Detection of incomplete fan-out operations on resume
  - Re-evaluation of child execution status
  - Continuation of pending child workflows
  - Preservation of completed child results

#### 3.4 Deep Synchronization Tests
- **Purpose:** Verify complex fan-out scenarios work correctly
- **Test Coverage:**
  - Multi-level fan-out with nested child executions
  - Cycle detection and graceful failure
  - Resume functionality for interrupted fan-outs
  - Timeout handling with partial completion
  - Deep synchronization with complex dependency graphs

**Acceptance Criteria:**
- Multi-level fan-out operations create proper hierarchical state
- Cycle detection prevents infinite recursion in event propagation
- Resume functionality correctly continues interrupted fan-out operations
- Deep synchronization waits for complete execution subtrees
- Timeout handling prevents indefinite blocking

---

### Phase 4: Advanced Features and Error Handling
**Goal:** Add configurable error handling, advanced concurrency, and production features

#### 4.1 Configurable Error Handling
- **File:** `internal/steps/fanout.go` (enhancement)
- **Purpose:** Add fail-fast configuration and error aggregation
- **Features:**
  - `fail-fast` parameter support (default: false)
  - Detailed error aggregation and reporting
  - Partial success handling and result collection
  - Clear error messages for debugging

#### 4.2 Advanced Concurrency Control
- **File:** `internal/steps/fanout.go` (enhancement)
- **Purpose:** Sophisticated concurrency management
- **Features:**
  - Resource-aware concurrency limiting
  - Dynamic concurrency adjustment based on system load
  - Priority-based child execution ordering
  - Deadlock detection and prevention

#### 4.3 Performance Optimizations
- **File:** `internal/engine/discovery.go` (enhancement)
- **Purpose:** Optimize discovery and execution performance
- **Features:**
  - Subscription caching and indexing
  - Batch repository operations
  - Lazy loading of subscription data
  - Performance metrics and monitoring

#### 4.4 Production Readiness
- **Files:** Various enhancements
- **Purpose:** Production-ready features and observability
- **Features:**
  - Comprehensive logging and metrics
  - Debug mode with detailed execution traces
  - Resource usage monitoring and alerts
  - Documentation and examples

#### 4.5 Comprehensive Testing
- **Purpose:** End-to-end testing of all fan-out scenarios
- **Test Coverage:**
  - Complex multi-repository fan-out workflows
  - Error scenarios and recovery testing
  - Performance testing with large subscription sets
  - Integration testing with existing engine components
  - End-to-end testing with real repository scenarios

**Acceptance Criteria:**
- Configurable error handling works for different failure scenarios
- Advanced concurrency features improve performance and resource usage
- Production features provide adequate observability and debugging
- Comprehensive testing covers all supported fan-out scenarios
- Performance meets requirements for large-scale operations

---

## Testing Strategy

### Unit Testing
- **Coverage Target:** >70% overall, >80% for new components
- **Focus Areas:**
  - Discovery manager subscription lookup and filtering
  - Fan-out step parameter validation and execution
  - State management hierarchical operations
  - Error handling and edge cases

### Integration Testing  
- **Multi-Repository Scenarios:** Use existing e2e test infrastructure
- **Event Flow Testing:** End-to-end event emission and subscription
- **State Persistence:** Resume functionality across restart boundaries
- **Concurrency Testing:** Parallel execution with resource constraints

### Performance Testing
- **Large Subscription Sets:** Test with hundreds of subscribed repositories
- **Deep Synchronization:** Multi-level fan-out with complex trees
- **Concurrent Operations:** Multiple fan-out steps in single workflow
- **Resource Usage:** Memory and CPU usage under load

## Risk Mitigation

### Technical Risks
1. **Complexity Management:** Phased implementation prevents overwhelming complexity
2. **State Consistency:** Comprehensive testing of hierarchical state management
3. **Performance Impact:** Performance testing and optimization in Phase 4
4. **Integration Issues:** Early integration testing in each phase

### Implementation Risks
1. **Scope Creep:** Strict adherence to phase boundaries and acceptance criteria
2. **Test Coverage:** Mandatory coverage checks before phase completion
3. **Breaking Changes:** Careful integration with existing engine components
4. **Documentation:** Inline documentation and examples throughout implementation

## Success Metrics

### Functional Success
- All fan-out step parameters work as specified
- Event emission and subscription evaluation function correctly
- Deep synchronization handles complex multi-level scenarios
- Resume functionality restores interrupted operations properly

### Quality Success
- Test coverage remains above 70% throughout implementation
- No regression in existing functionality
- Clear error messages and debugging information
- Comprehensive documentation and examples

### Performance Success
- Fan-out operations scale to hundreds of repositories
- Concurrency limits prevent resource exhaustion
- Discovery operations complete within reasonable time bounds
- Memory usage remains bounded for large operations

This implementation plan provides a clear roadmap for implementing the `tako/fan-out@v1` step while maintaining code quality, test coverage, and system stability throughout the development process.