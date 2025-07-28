# Implementation Plan for Issue #106: Subscription-Based Workflow Triggering

## Overview
Based on code analysis and Gemini consultation, the work is primarily **integration** rather than creating new files. The core components exist but need to be connected with additional features for idempotency, diamond dependency resolution, and performance optimization.

## Implementation Phases

### Phase 1: Idempotency Implementation
**Goal**: Implement at-least-once delivery with idempotency checking

**Tasks**:
1. Extend `FanOutState` struct to track triggered workflows
   - Add `TriggeredWorkflows map[string]string` field to track `subscriberRepo/workflow` → `runID`
   - Update state persistence/loading logic

2. Implement idempotency checking in `FanOutExecutor.triggerSubscribersWithState`
   - Check if workflow already triggered before processing
   - Skip already-triggered workflows with appropriate logging
   - Record successful triggers in state

**Files Modified**:
- `internal/engine/fanout_state.go` (add field to struct)
- `internal/engine/fanout.go` (add idempotency logic)

**Tests**:
- Unit tests for idempotency checking behavior
- Test that duplicate events don't retrigger workflows
- Test state persistence across restarts

### Phase 2: Diamond Dependency Resolution  
**Goal**: Implement first-subscription-wins policy for conflicting subscriptions

**Tasks**:
1. Add conflict detection in `FanOutExecutor.Execute`
   - Group `SubscriptionMatch` results by repository
   - Detect when multiple subscriptions match same event in same repo

2. Implement resolution logic
   - Apply "first-subscription-wins" deterministically
   - Ensure consistent ordering (alphabetical by repo, then by subscription order)
   - Log conflicts and resolution decisions

**Files Modified**:
- `internal/engine/fanout.go` (add resolution logic in Execute method)

**Tests**:
- Test scenarios with multiple subscriptions per repository
- Verify deterministic conflict resolution
- Test logging of resolution decisions

### Phase 3: Performance Optimizations
**Goal**: Cache compiled CEL expressions for better performance

**Tasks**:
1. Add CEL program caching to `SubscriptionEvaluator`
   - Add `sync.Map` for thread-safe cache
   - Implement cache lookup before compilation
   - Store compiled programs for reuse

2. Optimize repository discovery
   - Add caching for repository scanning results
   - Implement lazy loading of subscriptions

**Files Modified**:  
- `internal/engine/subscription.go` (add CEL caching)
- `internal/engine/discovery.go` (optional: repository scan caching)

**Tests**:
- Performance tests showing improvement with caching
- Concurrency tests for thread-safe cache access
- Test cache invalidation scenarios

### Phase 4: Workflow Triggering Integration
**Goal**: Connect subscription evaluation to actual workflow execution

**Tasks**:
1. Replace `simulateWorkflowTrigger` with real implementation
   - Execute `tako run` command in subscriber repositories
   - Pass inputs from subscription mapping
   - Handle execution errors and timeouts

2. Improve error handling and logging
   - Structured error reporting for failed triggers
   - Capture and log workflow execution output
   - Implement proper cleanup on failures

**Files Modified**:
- `internal/engine/fanout.go` (replace simulation with real triggering)

**Tests**:  
- Integration tests with actual workflow execution
- Error handling tests for failed workflows
- Input mapping validation tests

### Phase 5: Schema Compatibility Enhancement
**Goal**: Robust schema validation between event producers and consumers

**Tasks**:
1. Enhance schema compatibility checking
   - Improve semver range validation
   - Add detailed compatibility error messages
   - Implement schema evolution guidelines

2. Add schema registry support (if needed)
   - Optional: centralized schema management
   - Schema version negotiation

**Files Modified**:
- `internal/engine/subscription.go` (enhance compatibility logic)
- `internal/engine/event_model.go` (schema validation improvements)

**Tests**:
- Comprehensive schema compatibility test matrix
- Test edge cases in version ranges
- Test error reporting for incompatible schemas

## Success Criteria

### Functional Requirements
- ✅ Subscription filters evaluated correctly with CEL expressions
- ✅ Schema compatibility validated between event producers and consumers  
- ✅ Idempotency prevents duplicate workflow executions
- ✅ Diamond dependencies resolved with first-subscription-wins policy
- ✅ Performance acceptable for typical dependency trees

### Technical Requirements
- All tests pass (unit, integration, e2e)
- Test coverage maintained at ≥71% (allowing 1% drop from baseline)
- No individual function drops >10% in coverage
- Clean code with proper error handling and logging

## Phase Completion Criteria

Each phase must:
1. ✅ Compile without errors
2. ✅ Pass all existing tests
3. ✅ Add comprehensive tests for new functionality
4. ✅ Maintain or improve test coverage
5. ✅ Include proper logging and error handling
6. ✅ Update documentation as needed

## Dependencies and Assumptions

- Existing fan-out infrastructure from issue #105 is stable
- CEL library performance is acceptable with caching
- `tako run` command supports the required input parameters
- Repository caching structure remains consistent

## Risk Mitigation

- **Risk**: Real workflow triggering may introduce timeouts
  - **Mitigation**: Implement proper timeout handling and async execution
  
- **Risk**: CEL expression cache may consume excessive memory  
  - **Mitigation**: Implement cache size limits and LRU eviction

- **Risk**: Diamond dependency detection may be complex
  - **Mitigation**: Start with simple first-wins policy, enhance later if needed

## Testing Strategy

- **Unit Tests**: Each component tested in isolation
- **Integration Tests**: End-to-end subscription processing flow
- **Performance Tests**: CEL caching effectiveness
- **E2E Tests**: Full workflow triggering in test repositories