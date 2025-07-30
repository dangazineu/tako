# Issue #132 Implementation Plan

## Overview
Wire up the `tako/fan-out@v1` step to use the Orchestrator for discovery mechanism, ensuring proper logging of discovered subscriptions.

## Phase 1: Create Orchestrator instance in Runner
**Goal**: Add Orchestrator as a field in Runner struct and initialize it properly

**Tasks**:
1. Add `orchestrator *Orchestrator` field to Runner struct
2. Modify NewRunner to create and inject Orchestrator with DiscoveryManager
3. Update all NewRunner call sites if needed
4. Add tests for Runner initialization with Orchestrator

**Testing**: 
- Ensure all existing runner tests pass
- Add test to verify Orchestrator is properly initialized

## Phase 2: Modify executeFanOutStep to use Orchestrator
**Goal**: Call Orchestrator.DiscoverSubscriptions and log discovered subscriptions

**Tasks**:
1. Extract artifact from sourceRepo (format: "owner/repo:default")
2. Extract event_type from step parameters
3. Call Orchestrator.DiscoverSubscriptions with context
4. Log discovered subscriptions with appropriate detail
5. Handle errors from discovery appropriately

**Testing**:
- Add unit test for executeFanOutStep with mocked Orchestrator
- Test logging of discovered subscriptions
- Test error handling from discovery

## Phase 3: Pass discovered subscriptions to FanOutExecutor
**Goal**: Modify FanOutExecutor to accept pre-discovered subscriptions instead of discovering them itself

**Tasks**:
1. Add new method to FanOutExecutor: ExecuteWithSubscriptions
2. Modify existing Execute method to maintain backward compatibility
3. Update executeFanOutStep to use ExecuteWithSubscriptions
4. Remove direct discovery from FanOutExecutor when using ExecuteWithSubscriptions

**Testing**:
- Test new ExecuteWithSubscriptions method
- Ensure backward compatibility of Execute method
- Verify FanOutExecutor uses provided subscriptions

## Phase 4: Integration testing
**Goal**: Ensure the complete flow works end-to-end

**Tasks**:
1. Create integration test with real Orchestrator and FanOutExecutor
2. Test with various subscription scenarios
3. Verify logging output contains discovered subscriptions
4. Test error scenarios (no subscriptions, discovery errors)

**Testing**:
- Full integration test of fan-out step execution
- Verify logs contain expected subscription information
- Test coverage remains above baseline

## Phase 5: Documentation and cleanup
**Goal**: Ensure code is well-documented and clean

**Tasks**:
1. Add/update comments for modified functions
2. Update any relevant documentation
3. Ensure all tests pass with race detector
4. Run all linters and fix any issues

**Testing**:
- All tests pass: unit, integration, e2e
- Linters pass
- Coverage meets requirements

## Success Criteria
1. executeBuiltinStep correctly routes tako/fan-out@v1 to executeFanOutStep ✓ (already done)
2. executeFanOutStep uses Orchestrator to discover subscriptions
3. Discovered subscriptions are properly logged
4. All tests pass with good coverage
5. The fan-out step works in actual workflows