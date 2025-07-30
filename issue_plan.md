# Implementation Plan for Issue #131: Implement Orchestrator DiscoverSubscriptions

## Overview

Implement the `Orchestrator` component in `internal/engine/orchestrator.go` with a `DiscoverSubscriptions` method that coordinates subscription discovery without triggering workflows. This is part of the larger subscription-based workflow triggering system (#106).

## Phase-by-Phase Implementation

Each phase must leave the codebase in a healthy state (compiling + passing tests).

### Phase 1: Create Orchestrator Structure and Constructor
**Goal**: Establish the basic `Orchestrator` component with dependency injection

**Tasks**:
1. Create `internal/engine/orchestrator.go`
2. Define `Orchestrator` struct with `SubscriptionDiscoverer` dependency
3. Implement `NewOrchestrator` constructor
4. Add package documentation
5. Create basic compilation test

**Expected State**: 
- New file compiles without errors
- Constructor works with dependency injection
- No functional logic yet

**Tests**: Basic constructor test to verify dependency is set correctly

### Phase 2: Implement DiscoverSubscriptions Method
**Goal**: Add the core `DiscoverSubscriptions` method as pass-through to discoverer

**Tasks**:
1. Implement `DiscoverSubscriptions(ctx context.Context, artifact, eventType string) ([]interfaces.SubscriptionMatch, error)`
2. Initial implementation: simple pass-through to `discoverer.FindSubscribers`
3. Add method documentation explaining current and future purpose
4. Update coverage tracking

**Expected State**:
- Method works correctly as pass-through
- All existing tests still pass
- Method signature matches design decisions

**Tests**: Unit tests for happy path and error handling using mocked discoverer

### Phase 3: Comprehensive Unit Testing
**Goal**: Add complete test coverage for the Orchestrator

**Tasks**:
1. Create `internal/engine/orchestrator_test.go`
2. Implement `MockSubscriptionDiscoverer` for testing
3. Add tests for:
   - Constructor with valid dependency
   - DiscoverSubscriptions happy path
   - DiscoverSubscriptions error handling
   - Parameter validation
   - Context handling
4. Verify test coverage meets requirements

**Expected State**:
- High test coverage for new component
- All edge cases covered
- Mock infrastructure in place for future tests

**Tests**: Complete test suite achieving >90% coverage for orchestrator.go

### Phase 4: Integration and Documentation
**Goal**: Ensure integration with existing system and add comprehensive documentation

**Tasks**:
1. Add usage examples in code comments
2. Verify integration with existing `DiscoveryManager`
3. Add integration test demonstrating orchestrator + discovery manager
4. Update any relevant documentation
5. Run full test suite to ensure no regressions

**Expected State**:
- Orchestrator integrates seamlessly with existing components
- Clear documentation for future developers
- All tests passing with maintained coverage
- Ready for manual verification

**Tests**: Integration test showing orchestrator working with real DiscoveryManager

## Success Criteria

- [ ] `Orchestrator` component created in `internal/engine/orchestrator.go`
- [ ] `DiscoverSubscriptions` method correctly finds and returns subscription matches
- [ ] High unit test coverage (>90% for new code)
- [ ] Integration test with existing `DiscoveryManager`
- [ ] All existing tests continue to pass
- [ ] Test coverage maintained within 1% of baseline (76.7%)
- [ ] Code passes all linters and formatting checks
- [ ] No functional changes to existing behavior

## Risk Mitigation

- **Import Cycle Risk**: Using interfaces from `internal/interfaces` should prevent cycles
- **Breaking Changes**: Since this is new functionality, no existing code should be affected
- **Test Complexity**: Mock-based testing will keep tests fast and reliable
- **Performance**: Pass-through implementation ensures no performance impact

## Future Extensions (Not in This Issue)

The `Orchestrator` is designed to be extensible for future phases:
- Filtering subscriptions based on criteria
- Prioritizing subscriptions
- Adding logging and monitoring
- Coordinating workflow triggering
- Handling idempotency and state management

## Dependencies

- **Issue #130**: ✅ Completed - Foundational interfaces available
- **Existing Code**: `DiscoveryManager` and `SubscriptionDiscoverer` interface

## Testing Strategy Summary

1. **Unit Tests**: Mock all dependencies, test orchestrator logic in isolation
2. **Integration Tests**: Test with real `DiscoveryManager` to verify end-to-end flow
3. **Coverage**: Maintain overall project coverage within 1% of baseline
4. **Performance**: Ensure no performance regression vs. direct `DiscoveryManager` usage