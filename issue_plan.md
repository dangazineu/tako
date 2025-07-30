# Implementation Plan for Issue #131: Implement Orchestrator DiscoverSubscriptions

## Overview

Implement the `Orchestrator` component in `internal/engine/orchestrator.go` with a `DiscoverSubscriptions` method that coordinates subscription discovery without triggering workflows. This is part of the larger subscription-based workflow triggering system (#106).

## Phase-by-Phase Implementation

Each phase must leave the codebase in a healthy state (compiling + passing tests).

### Phase 1: Initial Implementation & Core Tests
**Goal**: Create the Orchestrator component with pass-through functionality and basic testing

**Tasks**:
1. Create `internal/engine/orchestrator.go` and `internal/engine/orchestrator_test.go`
2. Define `Orchestrator` struct with `SubscriptionDiscoverer` dependency
3. Implement `NewOrchestrator` constructor
4. Implement `DiscoverSubscriptions(ctx context.Context, artifact, eventType string) ([]interfaces.SubscriptionMatch, error)` as pass-through
5. Create `MockSubscriptionDiscoverer` in test file
6. Add core unit tests:
   - Constructor sets dependency correctly
   - DiscoverSubscriptions happy path (returns matches)
   - DiscoverSubscriptions error path (returns error)
7. Add basic package and method documentation

**Expected State**: 
- Complete working component with pass-through functionality
- Core functionality tested with mocked dependencies
- All tests passing

**Tests**: Core unit tests for constructor and basic DiscoverSubscriptions functionality

### Phase 2: Comprehensive Unit Testing & Documentation
**Goal**: Expand test coverage and add comprehensive documentation

**Tasks**:
1. Add expanded unit tests for:
   - Parameter validation (empty artifact, empty eventType)
   - Context handling (context cancellation)
   - Edge cases and boundary conditions
2. Enhance package and method documentation with:
   - Usage examples
   - Future extensibility notes
   - Clear explanation of orchestration purpose
3. Verify test coverage meets >90% target for new code
4. Update coverage tracking

**Expected State**:
- High test coverage for new component (>90%)
- All edge cases covered
- Comprehensive documentation for future developers

**Tests**: Complete test suite achieving >90% coverage for orchestrator.go

### Phase 3: Integration and Documentation
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
- [ ] Integration test successfully demonstrates Orchestrator using live DiscoveryManager to find known subscriptions
- [ ] All existing tests continue to pass
- [ ] Test coverage maintained within 1% of baseline (76.7%)
- [ ] Code passes all linters and formatting checks
- [ ] No functional changes to existing behavior

## Risk Mitigation

- **Import Cycle Risk**: Using interfaces from `internal/interfaces` should prevent cycles
- **Breaking Changes**: Since this is new functionality, no existing code should be affected
- **Test Complexity**: Mock-based testing will keep tests fast and reliable
- **Performance**: Pass-through implementation ensures no performance impact
- **Configuration Drift**: Future changes to `tako.yml` format could break DiscoveryManager (and by extension, Orchestrator). Integration test will catch such changes.

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