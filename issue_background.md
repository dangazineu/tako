# Issue #130 Background Research

## Issue Summary
Issue #130 is a foundational task for implementing a subscription-based workflow triggering system (issue #106). The task requires creating:
1. Two interfaces in `internal/interfaces`: `SubscriptionDiscoverer` and `WorkflowRunner`
2. Three components in `internal/steps/fanout.go`: `FanOutExecutor`, `FanOutStepParams`, and `FanOutStepResult`

## Parent Issue #106 Overview
The subscription-based workflow triggering system enables:
- Declarative, event-driven workflow orchestration across multiple repositories
- Lazy evaluation for repositories in dependency tree only
- At-least-once delivery with idempotency handling
- Diamond dependency resolution (first-subscription-wins)
- Schema compatibility validation

## Current Architecture Analysis

### Existing Components
1. **FanOut System** (`internal/engine/fanout.go`):
   - `FanOutExecutor` already exists in `internal/engine/fanout.go`
   - Handles execution of tako/fan-out@v1 steps
   - Integrates with discovery, subscription evaluation, state management, and monitoring

2. **Discovery System** (`internal/engine/discovery.go`):
   - `DiscoveryManager` handles repository discovery and subscription lookup
   - `FindSubscribers` method finds repositories subscribing to specific events
   - Works with cached repositories in the file system

3. **Subscription System** (`internal/engine/subscription.go`):
   - `SubscriptionEvaluator` handles event-subscription matching and filtering
   - Uses CEL (Common Expression Language) for filter expressions
   - Has security safeguards for CEL evaluation

4. **Event Model** (`internal/engine/event_model.go`):
   - Event validation and schema support
   - Event builder pattern implementation

### Key Observations
1. **No `internal/interfaces` directory exists** - needs to be created
2. **No `internal/steps` directory exists** - needs to be created
3. **FanOutExecutor already exists** in `internal/engine/fanout.go`, not in `internal/steps/fanout.go`
4. The system already has substantial infrastructure for event-driven workflows

### Integration Points
- The new interfaces will likely define contracts for the existing implementations
- The `internal/steps/fanout.go` components may be a refactoring or additional layer over existing functionality
- Need to understand how the new structure relates to existing code

## Dependencies and Related Work
- Config package has subscription validation (`internal/config/subscription.go`)
- Engine package has comprehensive fanout implementation
- No previous PRs attempting to resolve this issue were found

## Technical Considerations
1. **Interface Design**: Need to understand what methods the interfaces should expose
2. **Package Structure**: Creating new packages vs. using existing ones
3. **Backward Compatibility**: Ensure no breaking changes to existing functionality
4. **Testing**: Maintain or improve the 76.8% coverage baseline