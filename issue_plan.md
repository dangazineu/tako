# Issue #105 Implementation Plan: tako/fan-out@v1 Semantic Step

## Overview
This plan implements the `tako/fan-out@v1` built-in step through a phased approach, ensuring each phase leaves the codebase in a healthy, compiling, and test-passing state.

## Implementation Phases

### Phase 1: Core Repository Discovery Engine
**Goal**: Implement repository discovery with subscription matching
**Files to Create/Modify**:
- `internal/engine/discovery.go` (new)
- `internal/engine/discovery_test.go` (new)

**Functionality**:
- Repository discovery from filesystem or cache
- Load tako.yml configurations from discovered repositories  
- Match repositories with subscriptions to specific artifact:event combinations
- Basic subscription filter evaluation (CEL expressions)

**Acceptance Criteria**:
- [ ] Discover repositories in cache directory structure
- [ ] Load and parse tako.yml from each repository
- [ ] Match subscriptions by artifact reference (repo:artifact format)
- [ ] Evaluate basic CEL filters for event matching
- [ ] Unit tests with 90%+ coverage
- [ ] All existing tests continue to pass

### Phase 2: Subscription Evaluation Engine  
**Goal**: Implement comprehensive subscription evaluation with schema versioning
**Files to Create/Modify**:
- `internal/engine/subscription.go` (new)
- `internal/engine/subscription_test.go` (new)

**Functionality**:
- Schema version compatibility checking (semver ranges)
- Advanced CEL expression evaluation with event context
- Event payload validation against subscription filters
- Repository workflow mapping for triggered executions

**Acceptance Criteria**:
- [ ] Schema version compatibility using semver range matching
- [ ] Advanced CEL evaluation with full event context (.event.payload, etc.)
- [ ] Event payload filtering and validation
- [ ] Subscription-to-workflow mapping resolution
- [ ] Unit tests with 90%+ coverage
- [ ] Integration tests with mock events
- [ ] All existing tests continue to pass

### Phase 3: Basic Fan-Out Step Implementation
**Goal**: Implement core tako/fan-out@v1 step without deep synchronization
**Files to Create/Modify**:
- `internal/steps/fanout.go` (new)
- `internal/steps/fanout_test.go` (new)
- `internal/engine/runner.go` (modify executeBuiltinStep)

**Functionality**:
- Parse fan-out step parameters (event_type, wait_for_children, timeout, concurrency_limit)
- Event emission with schema versioning and payload
- Basic repository discovery integration
- Simple workflow triggering (fire-and-forget mode)

**Acceptance Criteria**:
- [ ] Parameter parsing and validation for tako/fan-out@v1
- [ ] Event emission with proper schema versioning
- [ ] Integration with discovery engine
- [ ] Basic workflow triggering in child repositories  
- [ ] Support for fire-and-forget mode (wait_for_children: false)
- [ ] Unit tests with 90%+ coverage
- [ ] Integration tests with multiple repositories
- [ ] All existing tests continue to pass

### Phase 4: Deep Synchronization with DFS Traversal
**Goal**: Implement waiting for complete execution trees with timeout handling
**Files to Create/Modify**:
- `internal/steps/fanout.go` (extend)
- `internal/engine/synchronization.go` (new)
- `internal/engine/synchronization_test.go` (new)

**Functionality**:
- DFS traversal of execution trees across repositories
- Wait for all triggered workflows to complete
- Timeout handling with partial results
- Execution tree state monitoring and aggregation

**Acceptance Criteria**:
- [ ] DFS traversal logic for execution tree completion
- [ ] Wait for all child executions when wait_for_children: true
- [ ] Timeout handling with configurable duration
- [ ] Execution state aggregation and reporting
- [ ] Partial failure handling and reporting
- [ ] Unit tests with complex execution trees
- [ ] Integration tests with timeouts and failures
- [ ] All existing tests continue to pass

### Phase 5: Concurrency Control and Resource Management
**Goal**: Implement concurrency limits and resource-aware execution
**Files to Create/Modify**:
- `internal/steps/fanout.go` (extend)
- `internal/engine/runner.go` (extend for concurrency)

**Functionality**:
- Concurrency limiting for parallel child executions
- Integration with existing resource management system
- Execution queue management with backpressure
- Resource quota validation across repositories

**Acceptance Criteria**:
- [ ] Concurrency limit enforcement (concurrency_limit parameter)
- [ ] Integration with ResourceManager for quota validation
- [ ] Execution queue with backpressure handling
- [ ] Resource-aware scheduling of child workflows
- [ ] Performance tests with high concurrency scenarios
- [ ] Unit tests for concurrency control logic
- [ ] All existing tests continue to pass

### Phase 6: Error Handling and Edge Cases
**Goal**: Comprehensive error handling and edge case coverage
**Files to Create/Modify**:
- All files from previous phases (extend error handling)
- `internal/steps/fanout_errors_test.go` (new)

**Functionality**:
- Repository not found handling
- Malformed tako.yml handling  
- Network/filesystem error recovery
- Invalid subscription filter handling
- Circular dependency detection

**Acceptance Criteria**:
- [ ] Graceful handling of missing repositories
- [ ] Error recovery for malformed configurations
- [ ] Network/filesystem error resilience
- [ ] Invalid CEL expression error reporting
- [ ] Circular dependency detection and prevention
- [ ] Comprehensive error testing suite
- [ ] Edge case integration tests
- [ ] All existing tests continue to pass

## Integration Points

### Existing Systems Integration
- **State Management**: Use existing ExecutionState for tracking child runs
- **Template Engine**: Leverage existing template context for event payloads
- **Lock Manager**: Use existing locks to prevent concurrent repository access
- **Container Manager**: Use existing containerized execution for child workflows
- **Resource Manager**: Integrate with existing resource quota management

### Configuration Integration
- **Built-in Step Registry**: Already configured in config.go:261
- **Event System**: Use existing Event and EventProduction structures
- **Subscription System**: Use existing Subscription validation and parsing

## Testing Strategy

### Unit Testing
- Each new file requires 90%+ test coverage
- Mock external dependencies (filesystem, network)
- Test error conditions and edge cases
- Performance testing for concurrency scenarios

### Integration Testing  
- Multi-repository fan-out scenarios
- Event emission and subscription matching
- Timeout and failure handling
- Resource limit validation

### E2E Testing
- Complete workflows with fan-out steps
- Real repository discovery and execution
- Performance testing with multiple repositories
- Error recovery scenarios

## Risk Mitigation

### Technical Risks
- **Performance**: Implement concurrency controls early
- **Resource Usage**: Integrate with existing resource management
- **Circular Dependencies**: Add detection in Phase 6
- **State Corruption**: Use existing proven state management

### Implementation Risks  
- **Breaking Changes**: Maintain backward compatibility
- **Test Coverage**: Enforce 90%+ coverage for new code
- **Integration Complexity**: Implement incremental integration
- **Edge Cases**: Dedicate full phase to error handling

## Success Criteria

### Functional Requirements
- [ ] Events emitted with correct schema versioning and payload
- [ ] Repository discovery finds all subscribed repositories  
- [ ] Subscription filter evaluation works with CEL expressions
- [ ] Deep synchronization waits for complete execution tree
- [ ] Timeout handling prevents indefinite waiting
- [ ] Concurrency limits prevent resource exhaustion

### Quality Requirements
- [ ] Overall test coverage maintained ≥71.9% (within 1% of baseline 72.9%)
- [ ] New code achieves 90%+ test coverage
- [ ] All existing unit and E2E tests continue to pass
- [ ] No performance regressions in existing workflows
- [ ] Clean, well-documented code following existing patterns

### Integration Requirements
- [ ] Seamless integration with existing execution engine
- [ ] Compatible with existing state management and workspace isolation
- [ ] Proper integration with resource and security management
- [ ] Maintains existing CLI interface and user experience