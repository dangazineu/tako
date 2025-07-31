# Issue #134: Implement Idempotency for Fan-Out State - Background Research

## Issue Context

### Current Issue (#134)
- **Goal**: Prevent duplicate workflow executions by introducing persistent state management
- **Parent Issue**: #106 - Implement subscription-based workflow triggering
- **Depends on**: #133 - Enable isolated child workflow execution (CLOSED)

### Key Requirements
1. A workflow is not triggered twice for the same event
2. State is persisted and reloaded correctly
3. Unit and integration tests for idempotency pass

## Existing Implementation Analysis

### Current State Management (`fanout_state.go`)
The file already exists with comprehensive state management functionality:

1. **Core Components**:
   - `FanOutState`: Tracks a fan-out operation and its child workflows
   - `ChildWorkflow`: Represents individual child workflow state
   - `FanOutStateManager`: Manages persistent state storage

2. **State Persistence**:
   - States are saved to JSON files in `<cacheDir>/fanout-states/`
   - States are loaded on startup from disk
   - Each fan-out operation gets a unique ID: `fanout-<timestamp>-<eventType>`

3. **Child Workflow Tracking**:
   - Children are identified by: `<repository>-<workflow>`
   - Status transitions: pending → running → completed/failed/timed_out
   - Tracks inputs, run IDs, and error messages

### Current Fan-Out Execution Flow (`fanout.go`)

1. **Fan-Out ID Generation**:
   ```go
   fanOutID := fmt.Sprintf("fanout-%d-%s", startTime.Unix(), params.EventType)
   ```
   - Based on timestamp and event type
   - Not truly unique for duplicate events

2. **Workflow Triggering**:
   - In `triggerSubscribersWithState`, child workflows are added to state before execution
   - Each child is executed in a goroutine with concurrency control
   - State is updated after each child completes

3. **Missing Idempotency**:
   - No check for duplicate event processing
   - Same event can trigger same workflows multiple times
   - No event deduplication mechanism

## Identified Gaps

### 1. Event Identification
- Current fan-out ID is timestamp-based, not event-based
- Need a deterministic way to identify duplicate events
- Should consider: event type, source, and payload

### 2. Duplicate Detection
- No mechanism to check if an event has been processed before
- Need to query existing states before creating new ones
- Should handle concurrent duplicate events

### 3. State Lookup
- Current implementation only retrieves states by exact ID
- Need ability to find states by event characteristics
- Should support efficient lookups for idempotency checks

## Related Code Patterns

### Event Model (`event_model.go`)
- Events have an ID field that could be used for deduplication
- Events include type, source, and payload
- Could generate deterministic IDs from event content

### Existing Tests
- Good test coverage for basic state operations
- Tests for child workflow status updates
- Tests for persistence and retrieval
- No tests for idempotency or duplicate handling

## Previous Work Analysis

### Issue #133 (Closed)
- Implemented isolated child workflow execution
- Added `ExecuteChildWorkflow` method to prevent deadlocks
- Establishes pattern for child workflow management

### Issue #106 (Parent)
- Requires at-least-once delivery with idempotency
- Mentions idempotency checking explicitly
- Part of larger subscription-based triggering system

## Design Considerations

### 1. Event Fingerprinting
- Generate deterministic ID from event properties
- Consider: event type, source repository, key payload fields
- Handle event variations that should be considered duplicates

### 2. State Querying
- Add methods to find states by event properties
- Index states efficiently for lookup
- Handle state cleanup for old events

### 3. Concurrent Safety
- Multiple fan-out executors might process same event
- Need atomic check-and-create operation
- Consider distributed locking if needed

### 4. Backward Compatibility
- Existing states without event fingerprints
- Migration path for existing deployments
- Maintain current API contracts

## Implementation Approach

Based on the research, the implementation should:

1. Add event fingerprinting to generate deterministic IDs
2. Implement duplicate detection before creating new states
3. Add state lookup methods for idempotency checks
4. Ensure thread-safe operations for concurrent events
5. Add comprehensive tests for idempotency scenarios
6. Maintain backward compatibility with existing states

## Questions and Uncertainties

### 1. Event Fingerprinting Strategy
- **Question**: What fields should be included in the event fingerprint to ensure uniqueness?
- **Options**:
  - Option A: Hash of (event_type + source + full_payload)
  - Option B: Hash of (event_type + source + event_id)
  - Option C: Use existing event.ID if available, fallback to hash
- **Concern**: Different payload representations of same logical event

### 2. State Storage and Lookup
- **Question**: Should we maintain an index file for efficient lookups?
- **Current**: States are stored as individual JSON files
- **Options**:
  - Option A: Add an index file mapping event fingerprints to state files
  - Option B: Scan all files on startup and maintain in-memory index
  - Option C: Use file naming convention with event fingerprint
- **Concern**: Performance impact of scanning many state files

### 3. Concurrent Event Handling
- **Question**: How to handle race conditions when multiple processes receive the same event?
- **Options**:
  - Option A: File-based locking using lock files
  - Option B: Atomic file operations with rename
  - Option C: Accept occasional duplicates and handle at child level
- **Concern**: Distributed systems may have clock skew

### 4. Idempotency Window
- **Question**: How long should we remember processed events?
- **Current**: CleanupCompletedStates can remove old states
- **Options**:
  - Option A: Keep all states until explicitly cleaned
  - Option B: Auto-cleanup after configurable duration (e.g., 24 hours)
  - Option C: Keep last N events per event type
- **Concern**: Storage growth vs duplicate protection

### 5. Backward Compatibility
- **Question**: How to handle existing states without event fingerprints?
- **Options**:
  - Option A: Ignore old states for idempotency checks
  - Option B: Generate fingerprints for existing states on load
  - Option C: Version the state format and migrate on access
- **Concern**: Existing deployments should not break

### 6. API Design
- **Question**: Should idempotency be opt-in or always enabled?
- **Options**:
  - Option A: Always check for duplicates (breaking change)
  - Option B: Add flag to fan-out parameters
  - Option C: Make it configurable at executor level
- **Concern**: Some use cases might want to allow duplicates

## Decisions (Based on Gemini's Recommendations)

### 1. Event Fingerprinting Strategy
**Decision**: Use existing event.ID if available, fallback to hash
- Primary: Use `event.ID` when present (from EnhancedEvent.Metadata.ID)
- Fallback: SHA256 hash of (event_type + source + normalized_payload)
- Rationale: Most reliable and backward compatible approach

### 2. State Storage and Lookup
**Decision**: Use file naming convention with event fingerprint
- Format: `fanout-<fingerprint>.json` for idempotent states
- Keep existing: `fanout-<timestamp>-<eventType>.json` for non-idempotent
- Rationale: Simple, performant, no separate index needed

### 3. Concurrent Event Handling
**Decision**: Atomic file operations with rename
- Write to temp file: `fanout-<fingerprint>.tmp.<random>`
- Atomic rename to: `fanout-<fingerprint>.json`
- Rationale: Standard pattern for distributed locking on shared filesystem

### 4. Idempotency Window
**Decision**: Auto-cleanup after configurable duration
- Default: 24 hours retention for completed states
- Configurable via FanOutStateManager
- Extend existing CleanupCompletedStates method
- Rationale: Balances storage growth with duplicate protection

### 5. Backward Compatibility
**Decision**: Ignore old states for idempotency checks
- New idempotency only applies to new events
- Old states remain functional but not checked
- Rationale: Safest approach with zero migration risk

### 6. API Design
**Decision**: Make it configurable at executor level
- Add `EnableIdempotency` field to FanOutExecutor (default: false)
- Can be enabled via configuration on initialization
- Rationale: Non-breaking change with opt-in adoption