# Background Research for Issue #131: Implement Orchestrator DiscoverSubscriptions

## Issue Overview

Issue #131 is part of implementing the larger feature from Issue #106 (subscription-based workflow triggering). The goal is to implement an `Orchestrator` component in `internal/engine/orchestrator.go` with a `DiscoverSubscriptions` method that can:

1. Find and parse `tako.yml` files
2. Match subscriptions based on artifact and event type
3. Identify repositories that should react to an event
4. NOT trigger workflows yet (that's for later phases)

## Parent Issue Context (#106)

The parent issue #106 aims to implement subscription-based workflow triggering with these key features:
- Lazy evaluation of repository dependencies
- At-least-once event delivery  
- Diamond dependency resolution
- Schema compatibility validation using CEL (Common Expression Language)
- Declarative event subscriptions across repositories

**Previous Attempt**: PR #128 attempted to implement the full #106 feature but was closed, likely because it was too large/complex for a single PR. The approach is now being broken down into smaller, focused issues.

## Dependencies

**Issue #130 (COMPLETED)**: Successfully implemented foundational components in PR #136 (merged):
- Created `internal/interfaces` package with `SubscriptionDiscoverer` and `WorkflowRunner` interfaces
- Created `internal/steps/fanout.go` with `FanOutStepExecutor`, `FanOutStepParams`, and `FanOutStepResult`
- Updated engine components to implement the new interfaces
- All tests passing, coverage maintained at 76.7%

## Existing Architecture Context

### Current Discovery Manager (`internal/engine/discovery.go`)
- Already implements `SubscriptionDiscoverer` interface
- Has `FindSubscribers(artifact, eventType string)` method
- Scans cached repositories for `tako.yml` files
- Matches subscriptions against artifact and event type
- Returns sorted `SubscriptionMatch` results

### Available Interfaces (from #130)
- `SubscriptionDiscoverer`: For finding repositories that subscribe to events
- `WorkflowRunner`: For executing workflows (will be used later)
- Shared types: `SubscriptionMatch`, `ExecutionResult`, `StepResult`

### Directory Structure
```
internal/engine/
├── discovery.go          # Already has subscription discovery logic
├── fanout.go             # Fan-out execution functionality
├── runner.go             # Workflow execution with adapter
└── [need to create] orchestrator.go  # New orchestrator component
```

## Integration Points

The new `Orchestrator` will need to:
1. Use the existing `DiscoveryManager` (which implements `SubscriptionDiscoverer`)
2. Coordinate subscription discovery across multiple repositories
3. Potentially add orchestration logic on top of basic discovery
4. Work with the existing `tako.yml` configuration format

## Subscription Configuration Format (from #106)
```yaml
subscriptions:
 - artifact: my-org/go-lib:go-lib
   events: [library_built]
   schema_version: "^1.0.0"
   filters:
     - semver.major(event.payload.version) > 0
   workflow: update_integration
```

## Previous Challenges (from PR #128)

The closed PR #128 showed that a full implementation was complex and included:
- Idempotency & state management
- CEL expression caching
- Schema compatibility validation
- Real workflow integration
- Thread safety concerns

Since #131 is specifically focused on just discovery (no workflow triggering), it should be much simpler.

## Key Technical Requirements

1. **Orchestrator Component**: Create `internal/engine/orchestrator.go`
2. **DiscoverSubscriptions Method**: Find repositories with matching subscriptions
3. **High Test Coverage**: Comprehensive unit tests
4. **No Workflow Triggering**: Discovery only, execution comes later
5. **Integration**: Work with existing `DiscoveryManager` and interfaces

## Approach Considerations

The `Orchestrator` could:
- **Option A**: Wrap the existing `DiscoveryManager` and add orchestration logic
- **Option B**: Be a higher-level component that coordinates multiple discovery managers
- **Option C**: Be a separate component that uses `SubscriptionDiscoverer` interface

Option A seems most appropriate since the core discovery logic already exists and works well.