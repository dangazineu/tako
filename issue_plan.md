# Issue #133 Implementation Plan

## Overview
Replace `simulateWorkflowTrigger()` with actual isolated child workflow execution to enable real child workflow triggering from `tako/fan-out@v1` steps.

## Implementation Phases

### Phase 1: Create Child Runner Factory 🏗️
**Goal:** Implement factory pattern for creating isolated child Runner instances

**Changes:**
- Create `internal/engine/child_runner_factory.go`
- Implement `ChildRunnerFactory` struct with workspace isolation
- Add methods: `NewChildRunnerFactory()`, `CreateChildRunner()`
- Ensure workspace path isolation: `<parent_workspace>/children/<child_run_id>`
- Share cache directory between parent and children

**Testing:**
- Unit tests for factory creation and workspace isolation
- Test that each child gets unique workspace directory
- Verify shared cache directory access

**Success Criteria:**
- Factory creates isolated Runner instances
- Workspace directories don't conflict
- All existing tests pass
- Code compiles and linter passes

---

### Phase 2: Create Child Workflow Executor 🚀
**Goal:** Implement component that uses factory to execute child workflows

**Changes:**
- Create `internal/engine/child_workflow_executor.go`
- Implement `ChildWorkflowExecutor` struct implementing `interfaces.WorkflowRunner`
- Add methods: `NewChildWorkflowExecutor()`, `RunWorkflow()` 
- Handle repository path resolution for child execution
- Implement proper error handling and cleanup with defer blocks

**Testing:**
- Unit tests for child workflow executor
- Test workflow execution with isolated workspaces
- Test error handling and cleanup scenarios
- Mock tests for workflow execution

**Success Criteria:**
- ChildWorkflowExecutor properly executes workflows in isolation
- Error handling works correctly
- Cleanup prevents workspace leaks
- All tests pass

---

### Phase 3: Wire Dependency Injection 🔌
**Goal:** Connect child executor to existing FanOutExecutor

**Changes:**
- Modify `internal/engine/runner.go` `executeFanOutStep()` method
- Create and inject `ChildRunnerFactory` into workflow execution
- Wire `ChildWorkflowExecutor` as `WorkflowRunner` interface
- Update `FanOutExecutor` to use injected executor instead of simulation

**Testing:**
- Integration tests for fan-out step with child execution
- Test that fan-out step creates and uses child runners
- Verify workspace isolation during concurrent execution
- Test error propagation from children to parent

**Success Criteria:**
- Fan-out step uses real child workflow execution
- Dependency injection works properly
- Integration tests pass
- No regression in existing functionality

---

### Phase 4: Replace Simulation with Real Execution 🔄
**Goal:** Remove simulation code and implement actual child workflow triggering

**Changes:**
- Replace `simulateWorkflowTrigger()` in `fanout.go` with real execution
- Update `triggerSubscribersWithState()` to call child executor
- Modify error collection to handle real child workflow errors
- Update logging to reflect actual execution vs simulation

**Testing:**
- End-to-end tests with actual child workflow execution
- Test concurrent child execution with semaphore limits
- Test error scenarios (child failures, timeouts)
- Performance testing with multiple child workflows

**Success Criteria:**
- Child workflows execute in separate repositories
- Concurrent execution works without conflicts
- Error handling properly aggregates child failures
- Performance is acceptable for typical use cases

---

### Phase 5: Enhance Error Collection and Cleanup 🧹
**Goal:** Improve error reporting and resource cleanup

**Changes:**
- Modify `FanOutResult` to include detailed error information
- Implement comprehensive cleanup for partial failures
- Add timeout handling for child workflow execution
- Update structured logging with execution details

**Testing:**
- Test error collection with multiple failed children
- Test cleanup when some children succeed and others fail
- Test timeout scenarios
- Test resource cleanup under various failure conditions

**Success Criteria:**
- Detailed error reporting from child workflows
- No resource leaks from failed executions
- Proper timeout handling
- Clean failure modes

---

## Phase Dependencies
```
Phase 1 (Factory) → Phase 2 (Executor) → Phase 3 (Wiring) → Phase 4 (Replace) → Phase 5 (Enhance)
```

## Testing Strategy per Phase
- **Unit Tests**: Each phase has isolated unit tests
- **Integration Tests**: Phase 3+ includes integration testing
- **E2E Tests**: Phase 4+ includes end-to-end validation
- **Performance Tests**: Phase 4 includes basic performance validation

## Coverage Requirements
- Maintain overall coverage ≥ 68.5% (max 1% drop from baseline 69.5%)
- New functions must have ≥ 80% coverage
- Critical paths (child execution, error handling) must have ≥ 95% coverage

## Rollback Strategy
Each phase is atomic and can be rolled back independently:
- Phase 1-2: Safe rollback, only new files added
- Phase 3: Rollback requires reverting runner.go changes
- Phase 4: Rollback requires restoring simulation behavior
- Phase 5: Rollback requires reverting result structure changes