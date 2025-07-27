# Issue #105 Implementation Plan: tako/fan-out@v1 Semantic Step

## Overview
This plan implements the `tako/fan-out@v1` built-in step through a phased approach, ensuring each phase leaves the codebase in a healthy, compiling, and test-passing state.

## Implementation Phases

### Phase 1: Subscription Discovery & Evaluation Engine
**Goal**: Implement unified repository discovery and subscription evaluation system
**Files to Create/Modify**:
- `internal/engine/orchestrator.go` (new)
- `internal/engine/orchestrator_test.go` (new)
- `internal/steps/` (new directory)

**Functionality**:
- Repository discovery from filesystem or cache
- Load tako.yml configurations from discovered repositories  
- Match repositories with subscriptions to specific artifact:event combinations
- Schema version compatibility checking (semver ranges)
- Advanced CEL expression evaluation with event context
- Event payload validation against subscription filters
- Repository workflow mapping for triggered executions

**Acceptance Criteria**:
- [ ] Discover repositories in cache directory structure
- [ ] Load and parse tako.yml from each repository
- [ ] Match subscriptions by artifact reference (repo:artifact format)
- [ ] Schema version compatibility using semver range matching
- [ ] Advanced CEL evaluation with full event context (.event.payload, etc.)
- [ ] Event payload filtering and validation
- [ ] Subscription-to-workflow mapping resolution
- [ ] Unit tests with 90%+ coverage
- [ ] Integration tests with mock events
- [ ] All existing tests continue to pass

### Phase 2: Basic Fan-Out Step Implementation
**Goal**: Implement core tako/fan-out@v1 step without deep synchronization
**Files to Create/Modify**:
- `internal/steps/fanout.go` (new)
- `internal/steps/fanout_test.go` (new)
- `internal/engine/runner.go` (modify executeBuiltinStep)

**Functionality**:
- Parse fan-out step parameters (event_type, wait_for_children, timeout, concurrency_limit)
- Event emission with schema versioning and payload
- Integration with orchestrator for repository discovery and subscription evaluation
- Simple workflow triggering (fire-and-forget mode)

**Acceptance Criteria**:
- [ ] Parameter parsing and validation for tako/fan-out@v1
- [ ] Event emission with proper schema versioning
- [ ] Integration with orchestrator engine
- [ ] Basic workflow triggering in child repositories  
- [ ] Support for fire-and-forget mode (wait_for_children: false)
- [ ] Unit tests with 90%+ coverage
- [ ] Integration tests with multiple repositories
- [ ] All existing tests continue to pass

### Phase 3: Deep Synchronization with DFS Traversal
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
- Parent ExecutionState tracking of child run IDs for unified status view

**Acceptance Criteria**:
- [ ] DFS traversal logic for execution tree completion
- [ ] Parent ExecutionState contains nested references to child run IDs
- [ ] Wait for all child executions when wait_for_children: true
- [ ] Timeout handling with configurable duration
- [ ] Execution state aggregation and reporting
- [ ] Partial failure handling and reporting
- [ ] Unit tests with complex execution trees
- [ ] Integration tests with timeouts and failures
- [ ] All existing tests continue to pass

### Phase 4: Error Handling and Edge Cases
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
- CEL expression complexity limits

**Acceptance Criteria**:
- [ ] Graceful handling of missing repositories
- [ ] Error recovery for malformed configurations
- [ ] Network/filesystem error resilience
- [ ] Invalid CEL expression error reporting with complexity limits
- [ ] Circular dependency detection and prevention
- [ ] Comprehensive error testing suite
- [ ] Edge case integration tests
- [ ] Documentation for efficient vs. inefficient CEL patterns
- [ ] All existing tests continue to pass

### Phase 5: Concurrency Control and Resource Management
**Goal**: Implement concurrency limits and resource-aware execution
**Files to Create/Modify**:
- `internal/steps/fanout.go` (extend)
- `internal/engine/orchestrator.go` (extend for concurrency)

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

### Phase 6: Final Integration & E2E Testing
**Goal**: Complete E2E testing with comprehensive scenarios
**Files to Create/Modify**:
- `test/e2e/templates/fanout-*` (new E2E test scenarios)
- Complete integration testing

**E2E Test Scenarios**:
- Simple Fan-Out: 1 parent, 2 children
- Diamond Dependency: A -> B, A -> C, B -> D, C -> D
- Deep Chain: A -> B -> C -> D
- Timeout Scenario: Child workflow hangs, tests parent timeout
- Circular Dependency: A -> B -> A (should detect and fail)
- Configuration Hell: Large numbers of repositories and subscriptions

**Acceptance Criteria**:
- [ ] All defined E2E scenarios implemented and passing
- [ ] Performance testing with large repository sets
- [ ] Memory and resource usage validation
- [ ] Complete integration with existing tako CLI
- [ ] Documentation and examples updated
- [ ] All existing tests continue to pass

## Integration Points

### Existing Systems Integration
- **State Management**: Use existing ExecutionState for tracking child runs with explicit parent-child run ID relationships
- **Template Engine**: Leverage existing template context for event payloads
- **Lock Manager**: Use existing locks to prevent concurrent repository access
- **Container Manager**: Use existing containerized execution for child workflows
- **Resource Manager**: Integrate with existing resource quota management
- **Orchestrator**: New centralized component to encapsulate multi-repository complexity

### Configuration Integration
- **Built-in Step Registry**: Already configured in config.go:261
- **Event System**: Use existing Event and EventProduction structures
- **Subscription System**: Use existing Subscription validation and parsing
- **Steps Directory**: New `internal/steps/` directory to organize all built-in step implementations

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
- **Performance**: Implement concurrency controls in Phase 5
- **Resource Usage**: Integrate with existing resource management
- **Circular Dependencies**: Add detection in Phase 4 (Error Handling)
- **State Corruption**: Use existing proven state management
- **Configuration Hell**: As repositories scale, managing subscriptions becomes complex
  - *Mitigation*: Document efficient subscription patterns; consider future `tako discover-subscriptions` command
- **CEL Expression Complexity**: Unconstrained CEL expressions could cause performance issues or debugging difficulties
  - *Mitigation*: Implement computational complexity limits; document efficient vs. inefficient CEL patterns

### Implementation Risks  
- **Breaking Changes**: Maintain backward compatibility
- **Test Coverage**: Enforce 90%+ coverage for new code
- **Integration Complexity**: Implement incremental integration with orchestrator pattern
- **Edge Cases**: Dedicate Phase 4 entirely to error handling and robustness

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