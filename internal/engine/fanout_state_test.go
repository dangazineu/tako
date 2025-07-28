package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFanOutStateManager(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "fanout-states")

	manager, err := NewFanOutStateManager(stateDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	if manager.stateDir != stateDir {
		t.Errorf("Expected state directory %s, got %s", stateDir, manager.stateDir)
	}

	// Verify directory was created
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Errorf("State directory was not created")
	}
}

func TestCreateAndGetFanOutState(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create a fan-out state
	id := "test-fanout-1"
	parentRunID := "parent-run-123"
	sourceRepo := "org/repo"
	eventType := "build_complete"
	timeout := 5 * time.Minute

	state, err := manager.CreateFanOutState(id, parentRunID, sourceRepo, eventType, true, timeout)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Verify state properties
	if state.ID != id {
		t.Errorf("Expected ID %s, got %s", id, state.ID)
	}
	if state.ParentRunID != parentRunID {
		t.Errorf("Expected parent run ID %s, got %s", parentRunID, state.ParentRunID)
	}
	if state.SourceRepo != sourceRepo {
		t.Errorf("Expected source repo %s, got %s", sourceRepo, state.SourceRepo)
	}
	if state.EventType != eventType {
		t.Errorf("Expected event type %s, got %s", eventType, state.EventType)
	}
	if state.Status != FanOutStatusPending {
		t.Errorf("Expected status %s, got %s", FanOutStatusPending, state.Status)
	}
	if !state.WaitingForAll {
		t.Errorf("Expected waiting for all to be true")
	}
	if state.Timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, state.Timeout)
	}

	// Retrieve the state
	retrievedState, err := manager.GetFanOutState(id)
	if err != nil {
		t.Fatalf("Failed to get fan-out state: %v", err)
	}

	if retrievedState.ID != id {
		t.Errorf("Retrieved state has wrong ID: expected %s, got %s", id, retrievedState.ID)
	}
}

func TestGetNonExistentFanOutState(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	_, err = manager.GetFanOutState("non-existent")
	if err == nil {
		t.Error("Expected error when getting non-existent state")
	}
}

func TestAddChildWorkflow(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	state, err := manager.CreateFanOutState("test-fanout", "", "org/repo", "build", true, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Add a child workflow
	repository := "target/repo1"
	workflow := "deploy"
	inputs := map[string]string{
		"version": "1.2.3",
		"env":     "staging",
	}

	child := state.AddChildWorkflow(repository, workflow, inputs)

	if child.Repository != repository {
		t.Errorf("Expected repository %s, got %s", repository, child.Repository)
	}
	if child.Workflow != workflow {
		t.Errorf("Expected workflow %s, got %s", workflow, child.Workflow)
	}
	if child.Status != ChildStatusPending {
		t.Errorf("Expected status %s, got %s", ChildStatusPending, child.Status)
	}
	if len(child.Inputs) != 2 {
		t.Errorf("Expected 2 inputs, got %d", len(child.Inputs))
	}
	if child.Inputs["version"] != "1.2.3" {
		t.Errorf("Expected version input '1.2.3', got '%s'", child.Inputs["version"])
	}

	// Verify child is in state
	childID := repository + "-" + workflow
	if _, exists := state.Children[childID]; !exists {
		t.Errorf("Child workflow not found in state")
	}
}

func TestUpdateChildStatus(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	state, err := manager.CreateFanOutState("test-fanout", "", "org/repo", "build", true, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Add children
	child1 := state.AddChildWorkflow("target/repo1", "deploy", map[string]string{})
	_ = state.AddChildWorkflow("target/repo2", "test", map[string]string{})

	// Start waiting
	state.StartWaiting()

	// Update first child to completed
	err = state.UpdateChildStatus("target/repo1", "deploy", ChildStatusCompleted, "run-123", "")
	if err != nil {
		t.Fatalf("Failed to update child status: %v", err)
	}

	if child1.Status != ChildStatusCompleted {
		t.Errorf("Expected child1 status %s, got %s", ChildStatusCompleted, child1.Status)
	}
	if child1.RunID != "run-123" {
		t.Errorf("Expected child1 run ID 'run-123', got '%s'", child1.RunID)
	}
	if child1.EndTime == nil {
		t.Errorf("Expected child1 end time to be set")
	}

	// State should still be waiting
	if state.Status != FanOutStatusWaiting {
		t.Errorf("Expected fan-out status %s, got %s", FanOutStatusWaiting, state.Status)
	}

	// Update second child to completed
	err = state.UpdateChildStatus("target/repo2", "test", ChildStatusCompleted, "run-456", "")
	if err != nil {
		t.Fatalf("Failed to update child status: %v", err)
	}

	// Now fan-out should be completed
	if state.Status != FanOutStatusCompleted {
		t.Errorf("Expected fan-out status %s, got %s", FanOutStatusCompleted, state.Status)
	}
	if state.EndTime == nil {
		t.Errorf("Expected fan-out end time to be set")
	}
}

func TestUpdateChildStatusWithFailure(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	state, err := manager.CreateFanOutState("test-fanout", "", "org/repo", "build", true, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Add children
	state.AddChildWorkflow("target/repo1", "deploy", map[string]string{})
	state.AddChildWorkflow("target/repo2", "test", map[string]string{})

	// Start waiting
	state.StartWaiting()

	// Update first child to completed
	err = state.UpdateChildStatus("target/repo1", "deploy", ChildStatusCompleted, "run-123", "")
	if err != nil {
		t.Fatalf("Failed to update child status: %v", err)
	}

	// Update second child to failed
	err = state.UpdateChildStatus("target/repo2", "test", ChildStatusFailed, "run-456", "Build failed")
	if err != nil {
		t.Fatalf("Failed to update child status: %v", err)
	}

	// Fan-out should be failed due to child failure
	if state.Status != FanOutStatusFailed {
		t.Errorf("Expected fan-out status %s, got %s", FanOutStatusFailed, state.Status)
	}
}

func TestFanOutStateTransitions(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	state, err := manager.CreateFanOutState("test-fanout", "", "org/repo", "build", false, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Initial state should be pending
	if state.Status != FanOutStatusPending {
		t.Errorf("Expected initial status %s, got %s", FanOutStatusPending, state.Status)
	}

	// Start fan-out
	err = state.StartFanOut()
	if err != nil {
		t.Fatalf("Failed to start fan-out: %v", err)
	}
	if state.Status != FanOutStatusRunning {
		t.Errorf("Expected status %s, got %s", FanOutStatusRunning, state.Status)
	}

	// Complete fan-out
	err = state.CompleteFanOut()
	if err != nil {
		t.Fatalf("Failed to complete fan-out: %v", err)
	}
	if state.Status != FanOutStatusCompleted {
		t.Errorf("Expected status %s, got %s", FanOutStatusCompleted, state.Status)
	}
	if state.EndTime == nil {
		t.Errorf("Expected end time to be set")
	}
	if !state.IsComplete() {
		t.Errorf("Expected IsComplete() to return true")
	}
}

func TestFanOutStateFailure(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	state, err := manager.CreateFanOutState("test-fanout", "", "org/repo", "build", false, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Fail fan-out
	errorMessage := "Network timeout"
	err = state.FailFanOut(errorMessage)
	if err != nil {
		t.Fatalf("Failed to fail fan-out: %v", err)
	}

	if state.Status != FanOutStatusFailed {
		t.Errorf("Expected status %s, got %s", FanOutStatusFailed, state.Status)
	}
	if state.ErrorMessage != errorMessage {
		t.Errorf("Expected error message '%s', got '%s'", errorMessage, state.ErrorMessage)
	}
	if !state.IsComplete() {
		t.Errorf("Expected IsComplete() to return true")
	}
}

func TestFanOutStateTimeout(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	state, err := manager.CreateFanOutState("test-fanout", "", "org/repo", "build", false, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Timeout fan-out
	err = state.TimeoutFanOut()
	if err != nil {
		t.Fatalf("Failed to timeout fan-out: %v", err)
	}

	if state.Status != FanOutStatusTimedOut {
		t.Errorf("Expected status %s, got %s", FanOutStatusTimedOut, state.Status)
	}
	if !state.IsComplete() {
		t.Errorf("Expected IsComplete() to return true")
	}
}

func TestGetSummary(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	state, err := manager.CreateFanOutState("test-fanout", "", "org/repo", "build", true, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Add children with different statuses
	state.AddChildWorkflow("target/repo1", "deploy", map[string]string{})
	state.AddChildWorkflow("target/repo2", "test", map[string]string{})
	state.AddChildWorkflow("target/repo3", "build", map[string]string{})

	state.UpdateChildStatus("target/repo1", "deploy", ChildStatusCompleted, "run-1", "")
	state.UpdateChildStatus("target/repo2", "test", ChildStatusFailed, "run-2", "Test failed")
	// repo3 remains pending

	summary := state.GetSummary()

	if summary.ID != "test-fanout" {
		t.Errorf("Expected summary ID 'test-fanout', got '%s'", summary.ID)
	}
	if summary.TotalChildren != 3 {
		t.Errorf("Expected total children 3, got %d", summary.TotalChildren)
	}
	if summary.CompletedChildren != 1 {
		t.Errorf("Expected completed children 1, got %d", summary.CompletedChildren)
	}
	if summary.FailedChildren != 1 {
		t.Errorf("Expected failed children 1, got %d", summary.FailedChildren)
	}
	if summary.PendingChildren != 1 {
		t.Errorf("Expected pending children 1, got %d", summary.PendingChildren)
	}
}

func TestStartWaitingWithNoChildren(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	state, err := manager.CreateFanOutState("test-fanout", "", "org/repo", "build", true, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Start waiting with no children
	err = state.StartWaiting()
	if err != nil {
		t.Fatalf("Failed to start waiting: %v", err)
	}

	// Should complete immediately since there are no children
	if state.Status != FanOutStatusCompleted {
		t.Errorf("Expected status %s, got %s", FanOutStatusCompleted, state.Status)
	}
	if state.EndTime == nil {
		t.Errorf("Expected end time to be set")
	}
}

func TestListActiveFanOuts(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create multiple fan-out states
	_, _ = manager.CreateFanOutState("fanout-1", "", "org/repo", "build", false, 0)
	state2, _ := manager.CreateFanOutState("fanout-2", "", "org/repo", "test", false, 0)
	_, _ = manager.CreateFanOutState("fanout-3", "", "org/repo", "deploy", false, 0)

	// Complete one of them
	state2.CompleteFanOut()

	active := manager.ListActiveFanOuts()

	if len(active) != 2 {
		t.Errorf("Expected 2 active fan-outs, got %d", len(active))
	}

	activeIDs := make(map[string]bool)
	for _, summary := range active {
		activeIDs[summary.ID] = true
	}

	if !activeIDs["fanout-1"] {
		t.Errorf("Expected fanout-1 to be active")
	}
	if !activeIDs["fanout-3"] {
		t.Errorf("Expected fanout-3 to be active")
	}
	if activeIDs["fanout-2"] {
		t.Errorf("Expected fanout-2 to not be active")
	}
}

func TestStatePersistence(t *testing.T) {
	tempDir := t.TempDir()

	// Create manager and state
	manager1, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	state1, err := manager1.CreateFanOutState("persistent-test", "parent-123", "org/repo", "build", true, 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	state1.AddChildWorkflow("target/repo1", "deploy", map[string]string{"env": "prod"})
	state1.StartFanOut()

	// Create new manager that should load existing states
	manager2, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create second state manager: %v", err)
	}

	// Retrieve the state
	state2, err := manager2.GetFanOutState("persistent-test")
	if err != nil {
		t.Fatalf("Failed to get persisted state: %v", err)
	}

	// Verify all properties were persisted
	if state2.ID != "persistent-test" {
		t.Errorf("Expected ID 'persistent-test', got '%s'", state2.ID)
	}
	if state2.ParentRunID != "parent-123" {
		t.Errorf("Expected parent run ID 'parent-123', got '%s'", state2.ParentRunID)
	}
	if state2.Status != FanOutStatusRunning {
		t.Errorf("Expected status %s, got %s", FanOutStatusRunning, state2.Status)
	}
	if len(state2.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(state2.Children))
	}

	// Verify child properties
	child := state2.Children["target/repo1-deploy"]
	if child == nil {
		t.Fatalf("Child workflow not found")
	}
	if child.Repository != "target/repo1" {
		t.Errorf("Expected child repository 'target/repo1', got '%s'", child.Repository)
	}
	if child.Inputs["env"] != "prod" {
		t.Errorf("Expected child input env 'prod', got '%s'", child.Inputs["env"])
	}
}

func TestCleanupCompletedStates(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create states
	state1, _ := manager.CreateFanOutState("old-completed", "", "org/repo", "build", false, 0)
	state2, _ := manager.CreateFanOutState("recent-completed", "", "org/repo", "test", false, 0)
	_, _ = manager.CreateFanOutState("active", "", "org/repo", "deploy", false, 0)

	// Complete states
	state1.CompleteFanOut()
	state2.CompleteFanOut()

	// Manually set old end time for state1
	oldTime := time.Now().Add(-2 * time.Hour)
	state1.EndTime = &oldTime

	// Cleanup states older than 1 hour
	err = manager.CleanupCompletedStates(1 * time.Hour)
	if err != nil {
		t.Fatalf("Failed to cleanup completed states: %v", err)
	}

	// state1 should be removed
	_, err = manager.GetFanOutState("old-completed")
	if err == nil {
		t.Errorf("Expected old-completed state to be removed")
	}

	// state2 should still exist (recent)
	_, err = manager.GetFanOutState("recent-completed")
	if err != nil {
		t.Errorf("Expected recent-completed state to still exist: %v", err)
	}

	// state3 should still exist (active)
	_, err = manager.GetFanOutState("active")
	if err != nil {
		t.Errorf("Expected active state to still exist: %v", err)
	}
}

func TestWorkflowIdempotency(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create a fan-out state
	state, err := manager.CreateFanOutState("idempotency-test", "", "org/repo", "build", false, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Initially, workflow should not be triggered
	isTriggered, runID := state.IsWorkflowTriggered("target/repo1", "deploy")
	if isTriggered {
		t.Errorf("Expected workflow not to be triggered initially")
	}
	if runID != "" {
		t.Errorf("Expected empty run ID initially, got '%s'", runID)
	}

	// Mark workflow as triggered
	testRunID := "run-12345"
	err = state.MarkWorkflowTriggered("target/repo1", "deploy", testRunID)
	if err != nil {
		t.Fatalf("Failed to mark workflow as triggered: %v", err)
	}

	// Check that workflow is now marked as triggered
	isTriggered, runID = state.IsWorkflowTriggered("target/repo1", "deploy")
	if !isTriggered {
		t.Errorf("Expected workflow to be marked as triggered")
	}
	if runID != testRunID {
		t.Errorf("Expected run ID '%s', got '%s'", testRunID, runID)
	}

	// Check that different workflow is not affected
	isTriggered, runID = state.IsWorkflowTriggered("target/repo1", "test")
	if isTriggered {
		t.Errorf("Expected different workflow not to be affected")
	}

	// Check that different repository is not affected
	isTriggered, runID = state.IsWorkflowTriggered("target/repo2", "deploy")
	if isTriggered {
		t.Errorf("Expected different repository not to be affected")
	}
}

func TestIdempotencyPersistence(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create a fan-out state and mark a workflow as triggered
	state, err := manager.CreateFanOutState("persistence-test", "", "org/repo", "build", false, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	testRunID := "run-persistence-123"
	err = state.MarkWorkflowTriggered("target/repo1", "deploy", testRunID)
	if err != nil {
		t.Fatalf("Failed to mark workflow as triggered: %v", err)
	}

	// Create new manager that should load existing states
	manager2, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create second state manager: %v", err)
	}

	// Retrieve the persisted state
	state2, err := manager2.GetFanOutState("persistence-test")
	if err != nil {
		t.Fatalf("Failed to get persisted state: %v", err)
	}

	// Verify idempotency state was persisted
	isTriggered, runID := state2.IsWorkflowTriggered("target/repo1", "deploy")
	if !isTriggered {
		t.Errorf("Expected workflow to be marked as triggered after persistence")
	}
	if runID != testRunID {
		t.Errorf("Expected persisted run ID '%s', got '%s'", testRunID, runID)
	}
}
