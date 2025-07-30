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
- **🔒 CRITICAL:** Implement cache locking mechanism using existing `LockManager`

**Testing:**
- Unit tests for factory creation and workspace isolation
- Test that each child gets unique workspace directory
- Verify shared cache directory access
- **🔒 NEW:** Test concurrent cache access to prevent race conditions

**Success Criteria:**
- Factory creates isolated Runner instances
- Workspace directories don't conflict
- Cache access is thread-safe with proper locking
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
- **📋 CRITICAL:** Implement `tako.yml` discovery within child workspace
- **🔄 CRITICAL:** Define input/payload passing from parent to child workflows
- Implement proper error handling and cleanup with defer blocks

**Testing:**
- Unit tests for child workflow executor
- Test workflow execution with isolated workspaces
- Test error handling and cleanup scenarios
- **📋 NEW:** Test missing/malformed `tako.yml` scenarios
- **🔒 NEW:** Security testing for path traversal vulnerabilities
- Mock tests for workflow execution

**Success Criteria:**
- ChildWorkflowExecutor properly executes workflows in isolation
- `tako.yml` discovery works reliably in child workspaces
- Security boundaries prevent workspace escape
- Error handling works correctly
- Cleanup prevents workspace leaks
- All tests pass

---

### Phase 3: Wire Dependency Injection 🔌
**Goal:** Connect child executor to existing FanOutExecutor

**Changes:**
- **🏗️ IMPROVED:** Create `ChildRunnerFactory` in `NewRunner()` instead of `executeFanOutStep()`
- Modify `internal/engine/runner.go` `executeFanOutStep()` method
- Wire `ChildWorkflowExecutor` as `WorkflowRunner` interface
- Update `FanOutExecutor` to use injected executor instead of simulation
- **🔧 CRITICAL:** Integrate with existing `ResourceManager` for resource limits

**Testing:**
- Integration tests for fan-out step with child execution
- Test that fan-out step creates and uses child runners
- Verify workspace isolation during concurrent execution
- Test error propagation from children to parent
- **🔧 NEW:** Verify other step types (shell, built-ins) remain unaffected

**Success Criteria:**
- Fan-out step uses real child workflow execution
- Dependency injection works properly at `NewRunner()` level
- Resource management applies to child workflows
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
- **🧹 CRITICAL:** Implement idempotent cleanup mechanism
- **🧹 CRITICAL:** Design orphan workspace reaper for abrupt terminations
- Add timeout handling for child workflow execution
- Update structured logging with execution details

**Testing:**
- Test error collection with multiple failed children
- Test cleanup when some children succeed and others fail
- Test timeout scenarios
- Test resource cleanup under various failure conditions
- **🧹 NEW:** Test abrupt parent process termination and subsequent cleanup
- **🧹 NEW:** Test idempotent cleanup (safe to run multiple times)

**Success Criteria:**
- Detailed error reporting from child workflows
- No resource leaks from failed executions
- Cleanup works even after abrupt termination
- Idempotent cleanup prevents double-cleanup errors
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

## Key Architectural Improvements (From Gemini Review)

### 🔒 Cache Locking Strategy
- Use existing `LockManager` to prevent race conditions in shared cache
- Lock granularity: per-repository to allow concurrent access to different repos
- Implement in Phase 1 as critical foundation

### 📋 Child Configuration Discovery
- Robust `tako.yml` location logic within child workspaces
- Graceful handling of missing/malformed configuration files
- Security validation to prevent workspace escape attempts

### 🔧 Resource Management Integration
- Connect `ChildWorkflowExecutor` with existing `ResourceManager`
- Implement configurable resource limits for child workflows
- Prevent system resource exhaustion from fan-out operations

### 🧹 Robust Cleanup Design
- Idempotent cleanup operations (safe to run multiple times)
- Orphan workspace reaper for recovery from abrupt terminations
- State tracking to determine what needs cleanup

## Risk Mitigation Strategies

### High-Risk Areas:
1. **Concurrent Cache Access** → Implement fine-grained locking
2. **Workspace Security** → Path traversal testing and validation
3. **Resource Exhaustion** → Integration with ResourceManager
4. **Orphaned Resources** → Idempotent cleanup and reaper design
5. **Dependency Injection** → Inject at `NewRunner()` level for cleaner architecture

## Rollback Strategy
Each phase is atomic and can be rolled back independently:
- Phase 1-2: Safe rollback, only new files added
- Phase 3: Rollback requires reverting runner.go changes
- Phase 4: Rollback requires restoring simulation behavior
- Phase 5: Rollback requires reverting result structure changes