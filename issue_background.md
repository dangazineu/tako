# Issue #133 Background Research

## Issue Summary
**Goal:** Enable isolated child workflow execution for the `tako/fan-out@v1` step to address the current deadlock issue where child workflows are only simulated.

## Current Architecture Analysis

### Current Flow
1. `Runner.executeFanOutStep()` receives a fan-out step
2. Uses `Orchestrator.DiscoverSubscriptions()` to find child repositories
3. Creates `FanOutExecutor` and calls `ExecuteWithSubscriptions()`
4. `triggerSubscribersWithState()` currently calls `simulateWorkflowTrigger()` - **THIS IS THE GAP**
5. `simulateWorkflowTrigger()` only prints simulation messages and sleeps 10ms

### Key Components
- **Runner** (`internal/engine/runner.go`): Main workflow executor
- **FanOutExecutor** (`internal/engine/fanout.go`): Handles fan-out operations  
- **Orchestrator** (`internal/engine/orchestrator.go`): Coordinates subscription discovery
- **DiscoveryManager**: Finds repositories with subscriptions

### Current Limitations
- Child workflows are **SIMULATED ONLY** (line 517 in fanout.go)
- No actual isolated execution environment for children
- The comment states: "This is a placeholder for Phase 2 - actual workflow triggering will be implemented in later phases"

## Parent Issue Context (#106)
- Implementing subscription-based workflow triggering system
- Evaluates event filters and maps events to workflows in child repositories
- Features: lazy evaluation, at-least-once delivery, diamond dependency resolution
- **Status:** This is the actual execution implementation phase

## Dependency Analysis (#132)
- **CLOSED** - Basic fan-out step execution is wired to discovery mechanism
- Fan-out step correctly logs discovered subscriptions
- Current PR #138 successfully wires the discovery mechanism

## Related Previous Work
- PR #127: Implemented `tako/fan-out@v1` semantic step - **MERGED**
- PR #128: Subscription-based workflow triggering - **CLOSED** 
- PR #126: Fan-out orchestration implementation - **CLOSED**
- Design doc #116: Tako Exec Workflow Engine - **MERGED**

## Architecture Context

### Integration Points
1. **Runner.executeBuiltinStep()** (line 522) - Entry point for fan-out
2. **FanOutExecutor.triggerSubscribersWithState()** (line 452) - Where child execution happens
3. **simulateWorkflowTrigger()** (line 517) - **TARGET FOR REPLACEMENT**

### Current Testing
- All tests passing (baseline coverage: 69.5%)
- E2E tests complete in 155.76s
- Integration tests verify discovery and simulation

### Key Requirements from Issue #133
1. **ExecuteChildWorkflow** method on `engine.Runner`
2. Create **new, separate Runner instance** for each child
3. **Isolated context** for each child workflow
4. Called by `FanOutExecutor` for each discovered subscription
5. End-to-end tests to verify functionality

## Deadlock Issue Analysis
The current simulation approach avoids actual execution, preventing:
- Real isolation testing
- Actual workflow execution validation  
- Performance and concurrency issue discovery
- Integration with real repository states

## Implementation Strategy
The gap is clear: replace `simulateWorkflowTrigger()` with actual child workflow execution through a new `ExecuteChildWorkflow` method on the Runner.

## Architectural Decisions (Resolved with Gemini)

### 1. Isolation Strategy: ✅ DECIDED
- **Create completely new Runner instances** for each child workflow
- **Separate workspace roots**: `<parent_workspace>/children/<child_run_id>`
- **Shared cache directory** to avoid re-downloading repositories
- **Factory pattern**: Create `ChildRunnerFactory` for clean instantiation

### 2. Integration Point: ✅ DECIDED  
- **Dependency injection approach**: FanOutExecutor receives WorkflowRunner interface
- **No ExecuteChildWorkflow on Runner**: Keep Runner focused on single workflow execution
- **Use existing FanOutStepExecutor** in `internal/steps/fanout.go` which already has the right interface

### 3. Context Isolation: ✅ DECIDED
- **Separate workspace directories**: Essential for file-level isolation
- **Independent state management**: Each child gets own ExecutionState 
- **Isolated locks**: Each child gets own LockManager directory
- **Shared template engine & container manager**: Thread-safe, can be shared

### 4. Error Handling: ✅ DECIDED
- **Fail parent on any child failure**: Fan-out step is atomic unit of work
- **Collect all errors**: Modify FanOutStepResult to include detailed error list
- **No failure threshold initially**: Start simple, add later if needed
- **Cleanup with defer blocks**: Ensure child workspaces are cleaned up

### 5. Concurrency: ✅ DECIDED
- **Goroutines with semaphore**: Control concurrency limit via semaphore
- **Workspace path isolation**: Key to preventing resource conflicts  
- **Concurrent cache access**: Handled by existing locking mechanisms

## Implementation Plan Summary
1. Create `ChildRunnerFactory` with workspace isolation
2. Create `ChildWorkflowExecutor` implementing `interfaces.WorkflowRunner`
3. Wire dependency injection through existing `FanOutStepExecutor`
4. Replace `simulateWorkflowTrigger()` with actual child workflow execution
5. Implement proper error collection and cleanup