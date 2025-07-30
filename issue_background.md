# Issue #132 Background Research

## Issue Summary
This task wires up the `tako/fan-out@v1` step to the discovery mechanism by modifying `executeBuiltinStep` in `internal/engine/runner.go` to handle the `tako/fan-out@v1` case.

## Parent Issue Context (#106)
- Implement subscription-based workflow triggering system
- Key features: lazy evaluation, at-least-once delivery, diamond dependency resolution, schema compatibility validation
- The system allows child repositories to declaratively specify which events they want to receive

## Dependencies
- Issue #131 (CLOSED): Implemented repository and subscription discovery
  - Created `Orchestrator` in `internal/engine/orchestrator.go`
  - Implemented `DiscoverSubscriptions` method

## Related Previous Work
- PR #127: Implemented `tako/fan-out@v1` semantic step for event-driven orchestration
  - Created comprehensive fan-out functionality with FanOutExecutor
  - Includes event emission, repository discovery, CEL filtering, semantic versioning
  - Has resilience patterns (circuit breakers, retry mechanisms)
  - State management and monitoring capabilities
  
- PR #136: Implemented foundational components for subscription-based triggering
  - Likely included the Orchestrator and DiscoveryManager components

## Current State Analysis
1. **executeBuiltinStep** in `runner.go`:
   - Already has a switch statement for built-in steps
   - Currently only handles `tako/fan-out@v1` case by calling `executeFanOutStep`
   - Returns error for unknown built-in steps

2. **executeFanOutStep** in `runner.go`:
   - Already implemented with basic functionality
   - Creates FanOutExecutor with cache directory and debug mode
   - Executes the fan-out step and handles state management
   - Coverage is low (45%) indicating it needs testing

3. **Orchestrator** in `orchestrator.go`:
   - Fully implemented with DiscoverSubscriptions method
   - Uses dependency injection pattern with SubscriptionDiscoverer interface
   - Has 100% test coverage

4. **DiscoveryManager** in `discovery.go`:
   - Implements SubscriptionDiscoverer interface
   - FindSubscribers method to find matching subscriptions
   - Has good test coverage (82.9%)

## Key Integration Points
1. The `executeBuiltinStep` already routes to `executeFanOutStep` for `tako/fan-out@v1`
2. The FanOutExecutor likely needs to use the Orchestrator to discover subscriptions
3. The integration should log discovered subscriptions as per acceptance criteria

## Architecture Understanding
```
executeBuiltinStep (router)
  └─> executeFanOutStep (handler)
      └─> FanOutExecutor (executor)
          └─> Orchestrator (discovery coordinator)
              └─> DiscoveryManager (actual discovery)
```

## Questions to Resolve
1. Does the current FanOutExecutor already use the Orchestrator for discovery?
   - **Answer**: No, FanOutExecutor uses DiscoveryManager directly
2. If not, how should we inject the Orchestrator into the FanOutExecutor?
   - **Answer**: We should NOT modify FanOutExecutor. Instead, modify executeFanOutStep to use Orchestrator and pass discovered subscriptions to FanOutExecutor
3. What logging format/level should be used for discovered subscriptions?
   - **Answer**: Will follow existing patterns in the codebase for fan-out logging
4. Are there any missing tests for the executeBuiltinStep routing logic?
   - **Answer**: Need to check and add tests for the orchestrator integration

## Architectural Decision
Based on analysis with Gemini:
- Orchestrator is meant to be the abstraction layer over DiscoveryManager
- executeFanOutStep should call Orchestrator.DiscoverSubscriptions
- Pass discovered subscriptions to FanOutExecutor
- Keep FanOutExecutor focused on execution, not discovery
- This maintains separation of concerns and follows the intended architecture