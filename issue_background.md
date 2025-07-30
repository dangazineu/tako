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

## Technical Questions and Resolution

### Key Architectural Questions Resolved with Gemini

1. **Architecture Design**: Should Orchestrator wrap DiscoveryManager or be separate?
   - **Decision**: Create separate component using `SubscriptionDiscoverer` interface
   - **Rationale**: Better separation of concerns, improved testability, and flexibility for future changes

2. **Method Signature**: What should DiscoverSubscriptions look like?
   - **Decision**: `DiscoverSubscriptions(ctx context.Context, artifact, eventType string) ([]interfaces.SubscriptionMatch, error)`
   - **Rationale**: Matches current FindSubscribers signature, provides stable high-level API

3. **Differentiation**: How should it differ from FindSubscribers?
   - **Purpose**: Acts as abstraction layer and future home for orchestration logic
   - **Initial**: Simple pass-through to discoverer
   - **Future**: Will add filtering, prioritization, logging, error handling

4. **Integration**: Use dependency injection with SubscriptionDiscoverer interface
   - **Decision**: Constructor accepts interface, not concrete implementation
   - **Benefits**: Loose coupling, testability, standard Go practice

5. **Testing Strategy**: Focus on orchestrator logic, not re-testing DiscoveryManager
   - **Approach**: Mock SubscriptionDiscoverer interface
   - **Focus**: Test orchestrator's coordination and error handling

## Implementation Approach

Create `internal/engine/orchestrator.go` with:
- `Orchestrator` struct with `SubscriptionDiscoverer` dependency
- `NewOrchestrator` constructor using dependency injection  
- `DiscoverSubscriptions` method as initial pass-through
- Comprehensive unit tests with mocked dependencies