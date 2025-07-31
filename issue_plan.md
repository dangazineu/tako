# Issue #134: Implementation Plan - Idempotency for Fan-Out State

## Overview
Implement idempotency for fan-out state management to prevent duplicate workflow executions when the same event is processed multiple times.

## Implementation Phases

### Phase 1: Add Event Fingerprinting
**Goal**: Create deterministic identifiers for events to enable duplicate detection

**Tasks**:
1. Add `GenerateEventFingerprint` method to `fanout_state.go`:
   - Use EnhancedEvent.Metadata.ID if available
   - Fallback to SHA256 hash of (type + source + normalized payload)
   - Handle both EnhancedEvent and legacy Event types

2. Add `normalizePayload` helper function:
   - Sort map keys for consistent hashing
   - Handle nested maps recursively
   - Convert to canonical JSON representation

3. Add unit tests for fingerprint generation:
   - Test with event ID present
   - Test fallback to hash
   - Test payload normalization
   - Test deterministic output

**Files to modify**:
- `internal/engine/fanout_state.go`
- `internal/engine/fanout_state_test.go`

### Phase 2: Add Idempotency Configuration
**Goal**: Make idempotency opt-in at the executor level

**Tasks**:
1. Add `EnableIdempotency` field to `FanOutExecutor` struct
2. Update `NewFanOutExecutor` to accept idempotency option
3. Add configuration method `SetIdempotency(enabled bool)`
4. Update constructor to maintain backward compatibility

**Files to modify**:
- `internal/engine/fanout.go`

### Phase 3: Implement State Lookup by Fingerprint
**Goal**: Enable efficient duplicate detection using file naming convention

**Tasks**:
1. Add `GetFanOutStateByFingerprint` method to `FanOutStateManager`:
   - Check for file existence using fingerprint-based name
   - Return existing state if found
   - Return nil if not found (not an error)

2. Update `CreateFanOutState` to support idempotent creation:
   - Add `fingerprint` parameter
   - Use fingerprint-based naming when provided
   - Keep timestamp-based naming for non-idempotent mode

3. Add `createStateAtomic` helper method:
   - Write to temporary file
   - Attempt atomic rename
   - Handle race conditions gracefully

**Files to modify**:
- `internal/engine/fanout_state.go`
- `internal/engine/fanout_state_test.go`

### Phase 4: Integrate Idempotency into Fan-Out Execution
**Goal**: Use fingerprinting and state lookup to prevent duplicate executions

**Tasks**:
1. Update `executeWithContextAndSubscriptions` in `fanout.go`:
   - Generate event fingerprint if idempotency enabled
   - Check for existing state before creating new one
   - Return early with existing state if found
   - Log duplicate detection for debugging

2. Handle existing state scenarios:
   - If state is complete: return its result
   - If state is running: optionally wait or return immediately
   - If state is failed: optionally retry or return failure

3. Update state creation calls:
   - Pass fingerprint when idempotency is enabled
   - Use traditional ID generation when disabled

**Files to modify**:
- `internal/engine/fanout.go`

### Phase 5: Enhance State Cleanup
**Goal**: Implement configurable retention for idempotent states

**Tasks**:
1. Add `IdempotencyRetention` field to `FanOutStateManager`
2. Update `CleanupCompletedStates` to respect retention policy:
   - Default to 24 hours for idempotent states
   - Keep existing behavior for non-idempotent states
   - Only cleanup states with fingerprint-based names

3. Add configuration method for retention period

**Files to modify**:
- `internal/engine/fanout_state.go`

### Phase 6: Add Comprehensive Tests
**Goal**: Ensure idempotency works correctly in various scenarios

**Tasks**:
1. Add idempotency tests to `fanout_test.go`:
   - Test duplicate event detection
   - Test concurrent duplicate events
   - Test with and without event IDs
   - Test configuration toggle

2. Add integration tests:
   - Test end-to-end duplicate prevention
   - Test state persistence and recovery
   - Test cleanup behavior

3. Add benchmark tests:
   - Measure fingerprint generation performance
   - Measure lookup performance with many states

**Files to modify**:
- `internal/engine/fanout_test.go`
- `internal/engine/fanout_state_test.go`

### Phase 7: Update Documentation
**Goal**: Document the new idempotency feature

**Tasks**:
1. Update code comments and godoc
2. Add examples of enabling idempotency
3. Document configuration options
4. Explain duplicate detection behavior

**Files to modify**:
- `internal/engine/fanout.go`
- `internal/engine/fanout_state.go`
- `README.md` (if applicable)

## Testing Strategy

### Unit Tests
- Event fingerprint generation
- State lookup by fingerprint
- Atomic file operations
- Configuration options

### Integration Tests
- End-to-end duplicate prevention
- Concurrent event handling
- State persistence and recovery
- Cleanup with retention

### Manual Testing
- Create test scenario with duplicate events
- Verify only one execution per event
- Test with high concurrency
- Verify backward compatibility

## Rollback Plan
If issues arise:
1. Disable idempotency via configuration (default is disabled)
2. Revert to previous version if critical issues
3. States created with fingerprints can coexist with timestamp-based states

## Success Criteria
1. Duplicate events do not trigger duplicate workflows
2. All existing tests continue to pass
3. Performance impact is minimal (<5% overhead)
4. Backward compatibility is maintained
5. Coverage remains above baseline levels