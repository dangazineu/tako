package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewExecutionState(t *testing.T) {
	tempDir := t.TempDir()
	runID := "exec-20240726-143022-a7b3c1d2"

	state, err := NewExecutionState(runID, tempDir)
	if err != nil {
		t.Fatalf("Failed to create execution state: %v", err)
	}

	if state.RunID != runID {
		t.Errorf("Expected run ID %s, got %s", runID, state.RunID)
	}

	if state.Status != StatusPending {
		t.Errorf("Expected initial status to be pending, got %s", state.Status)
	}

	if state.Steps == nil {
		t.Error("Steps map should be initialized")
	}

	if state.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", state.Version)
	}

	// Check that state directory was created
	stateDir := filepath.Join(tempDir, "state")
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("State directory should be created")
	}
}

func TestExecutionStateLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	runID := "exec-20240726-143022-a7b3c1d2"

	state, err := NewExecutionState(runID, tempDir)
	if err != nil {
		t.Fatalf("Failed to create execution state: %v", err)
	}

	// Start execution
	workflowName := "test-workflow"
	repository := "/path/to/repo"
	inputs := map[string]string{"env": "dev"}

	err = state.StartExecution(workflowName, repository, inputs)
	if err != nil {
		t.Fatalf("Failed to start execution: %v", err)
	}

	if state.Status != StatusRunning {
		t.Errorf("Expected status to be running, got %s", state.Status)
	}

	if state.WorkflowName != workflowName {
		t.Errorf("Expected workflow name %s, got %s", workflowName, state.WorkflowName)
	}

	if state.Repository != repository {
		t.Errorf("Expected repository %s, got %s", repository, state.Repository)
	}

	// Complete execution
	err = state.CompleteExecution()
	if err != nil {
		t.Fatalf("Failed to complete execution: %v", err)
	}

	if state.Status != StatusCompleted {
		t.Errorf("Expected status to be completed, got %s", state.Status)
	}

	if state.EndTime == nil {
		t.Error("End time should be set after completion")
	}
}

func TestExecutionStatePersistence(t *testing.T) {
	tempDir := t.TempDir()
	runID := "exec-20240726-143022-a7b3c1d2"

	// Create and modify state
	state1, err := NewExecutionState(runID, tempDir)
	if err != nil {
		t.Fatalf("Failed to create execution state: %v", err)
	}

	err = state1.StartExecution("test-workflow", "/path/to/repo", map[string]string{"env": "dev"})
	if err != nil {
		t.Fatalf("Failed to start execution: %v", err)
	}

	// Load state from disk
	state2, err := LoadExecutionState(runID, tempDir)
	if err != nil {
		t.Fatalf("Failed to load execution state: %v", err)
	}

	if state2.Status != StatusRunning {
		t.Errorf("Loaded state should have running status, got %s", state2.Status)
	}

	if state2.WorkflowName != "test-workflow" {
		t.Errorf("Loaded state should have correct workflow name, got %s", state2.WorkflowName)
	}
}

func TestExecutionStateSteps(t *testing.T) {
	tempDir := t.TempDir()
	runID := "exec-20240726-143022-a7b3c1d2"

	state, err := NewExecutionState(runID, tempDir)
	if err != nil {
		t.Fatalf("Failed to create execution state: %v", err)
	}

	stepID := "test-step"

	// Start step
	err = state.StartStep(stepID)
	if err != nil {
		t.Fatalf("Failed to start step: %v", err)
	}

	if state.GetStepStatus(stepID) != StatusRunning {
		t.Errorf("Expected step status to be running, got %s", state.GetStepStatus(stepID))
	}

	if state.CurrentStep != stepID {
		t.Errorf("Expected current step to be %s, got %s", stepID, state.CurrentStep)
	}

	// Complete step
	output := "step output"
	outputs := map[string]string{"result": "success"}

	err = state.CompleteStep(stepID, output, outputs)
	if err != nil {
		t.Fatalf("Failed to complete step: %v", err)
	}

	if state.GetStepStatus(stepID) != StatusCompleted {
		t.Errorf("Expected step status to be completed, got %s", state.GetStepStatus(stepID))
	}

	stepOutputs := state.GetStepOutputs(stepID)
	if stepOutputs["result"] != "success" {
		t.Errorf("Expected step output 'result' to be 'success', got %s", stepOutputs["result"])
	}
}

func TestExecutionStateFailure(t *testing.T) {
	tempDir := t.TempDir()
	runID := "exec-20240726-143022-a7b3c1d2"

	state, err := NewExecutionState(runID, tempDir)
	if err != nil {
		t.Fatalf("Failed to create execution state: %v", err)
	}

	// Start execution
	err = state.StartExecution("test-workflow", "/path/to/repo", map[string]string{})
	if err != nil {
		t.Fatalf("Failed to start execution: %v", err)
	}

	// Fail execution
	errorMsg := "execution failed"
	err = state.FailExecution(errorMsg)
	if err != nil {
		t.Fatalf("Failed to fail execution: %v", err)
	}

	if state.Status != StatusFailed {
		t.Errorf("Expected status to be failed, got %s", state.Status)
	}

	if state.Error != errorMsg {
		t.Errorf("Expected error message %s, got %s", errorMsg, state.Error)
	}

	if state.EndTime == nil {
		t.Error("End time should be set after failure")
	}
}

func TestExecutionStateStepFailure(t *testing.T) {
	tempDir := t.TempDir()
	runID := "exec-20240726-143022-a7b3c1d2"

	state, err := NewExecutionState(runID, tempDir)
	if err != nil {
		t.Fatalf("Failed to create execution state: %v", err)
	}

	stepID := "failing-step"

	// Start step
	err = state.StartStep(stepID)
	if err != nil {
		t.Fatalf("Failed to start step: %v", err)
	}

	// Fail step
	errorMsg := "step failed"
	err = state.FailStep(stepID, errorMsg)
	if err != nil {
		t.Fatalf("Failed to fail step: %v", err)
	}

	if state.GetStepStatus(stepID) != StatusFailed {
		t.Errorf("Expected step status to be failed, got %s", state.GetStepStatus(stepID))
	}

	failedSteps := state.GetFailedSteps()
	if len(failedSteps) != 1 || failedSteps[0] != stepID {
		t.Errorf("Expected one failed step %s, got %v", stepID, failedSteps)
	}
}

func TestExecutionStateResumable(t *testing.T) {
	tempDir := t.TempDir()
	runID := "exec-20240726-143022-a7b3c1d2"

	state, err := NewExecutionState(runID, tempDir)
	if err != nil {
		t.Fatalf("Failed to create execution state: %v", err)
	}

	// Initially not resumable
	if state.IsResumable() {
		t.Error("New execution should not be resumable")
	}

	// Start and fail a step
	err = state.StartStep("step1")
	if err != nil {
		t.Fatalf("Failed to start step: %v", err)
	}

	err = state.FailStep("step1", "error")
	if err != nil {
		t.Fatalf("Failed to fail step: %v", err)
	}

	// Fail the execution
	err = state.FailExecution("step failed")
	if err != nil {
		t.Fatalf("Failed to fail execution: %v", err)
	}

	// Now it should be resumable
	if !state.IsResumable() {
		t.Error("Failed execution with failed steps should be resumable")
	}
}

func TestExecutionStateSummary(t *testing.T) {
	tempDir := t.TempDir()
	runID := "exec-20240726-143022-a7b3c1d2"

	state, err := NewExecutionState(runID, tempDir)
	if err != nil {
		t.Fatalf("Failed to create execution state: %v", err)
	}

	// Start execution
	err = state.StartExecution("test-workflow", "/path/to/repo", map[string]string{})
	if err != nil {
		t.Fatalf("Failed to start execution: %v", err)
	}

	// Add some steps
	err = state.StartStep("step1")
	if err != nil {
		t.Fatalf("Failed to start step1: %v", err)
	}

	err = state.CompleteStep("step1", "output", nil)
	if err != nil {
		t.Fatalf("Failed to complete step1: %v", err)
	}

	err = state.StartStep("step2")
	if err != nil {
		t.Fatalf("Failed to start step2: %v", err)
	}

	err = state.FailStep("step2", "error")
	if err != nil {
		t.Fatalf("Failed to fail step2: %v", err)
	}

	// Get summary
	summary := state.GetExecutionSummary()

	if summary["run_id"] != runID {
		t.Errorf("Expected run_id %s in summary, got %v", runID, summary["run_id"])
	}

	if summary["status"] != StatusRunning {
		t.Errorf("Expected status running in summary, got %v", summary["status"])
	}

	steps, ok := summary["steps"].(map[string]int)
	if !ok {
		t.Fatal("Expected steps to be a map[string]int")
	}

	if steps["total"] != 2 {
		t.Errorf("Expected 2 total steps, got %d", steps["total"])
	}

	if steps["completed"] != 1 {
		t.Errorf("Expected 1 completed step, got %d", steps["completed"])
	}

	if steps["failed"] != 1 {
		t.Errorf("Expected 1 failed step, got %d", steps["failed"])
	}
}
