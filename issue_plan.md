# Implementation Plan for Issue #130

## Overview
Create foundational components for subscription-based triggering system with no functional changes.

## Phase 1: Create Interface Package and Definitions
**Goal**: Establish the interface contracts
**Files to create**:
- `internal/interfaces/subscription.go` - Contains SubscriptionDiscoverer interface
- `internal/interfaces/workflow.go` - Contains WorkflowRunner interface

**Actions**:
1. Create `internal/interfaces` directory
2. Define `SubscriptionDiscoverer` interface with `FindSubscribers` method
3. Define `WorkflowRunner` interface with `ExecuteWorkflow` method
4. Import necessary types from engine package

**Testing**: Ensure code compiles

## Phase 2: Create Steps Package Structure
**Goal**: Establish the steps package with fanout components
**Files to create**:
- `internal/steps/fanout.go` - Contains FanOutExecutor, FanOutStepParams, FanOutStepResult

**Actions**:
1. Create `internal/steps` directory
2. Create `fanout.go` with basic structure
3. Define `FanOutStepParams` struct (will be populated later)
4. Define `FanOutStepResult` struct (will be populated later)
5. Define `FanOutExecutor` struct with interface dependencies

**Testing**: Ensure code compiles

## Phase 3: Extract Types from Engine Package
**Goal**: Move FanOutParams and FanOutResult to steps package
**Files to modify**:
- `internal/engine/fanout.go` - Remove FanOutParams and FanOutResult
- `internal/steps/fanout.go` - Add FanOutStepParams and FanOutStepResult

**Actions**:
1. Copy `FanOutParams` from engine to steps as `FanOutStepParams`
2. Copy `FanOutResult` from engine to steps as `FanOutStepResult`
3. Update imports in engine package to use the new types
4. Ensure all references are updated

**Testing**: Run all tests to ensure no breakage

## Phase 4: Implement Interface Compliance
**Goal**: Ensure existing types implement the new interfaces
**Files to modify**:
- `internal/engine/discovery.go` - Ensure DiscoveryManager implements SubscriptionDiscoverer
- `internal/engine/runner.go` - Ensure Runner implements WorkflowRunner

**Actions**:
1. Add any necessary method adjustments (should be minimal)
2. Verify interface satisfaction with compile-time checks
3. Add comments indicating interface implementation

**Testing**: Run all tests, check coverage

## Phase 5: Final Integration and Documentation
**Goal**: Complete the foundational setup
**Files to modify**:
- Update any necessary imports
- Add package documentation

**Actions**:
1. Add package-level documentation for interfaces and steps packages
2. Ensure all new types have proper godoc comments
3. Run gofmt on all modified files

**Testing**: 
- Run full test suite
- Verify coverage hasn't dropped more than 1%
- Run linters

## Success Criteria
- All new packages and files created
- Code compiles without errors
- All tests pass
- Coverage remains within acceptable range (≥75.8%)
- No functional changes to existing behavior