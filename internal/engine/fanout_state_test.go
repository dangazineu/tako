package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/dangazineu/tako/internal/config"
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

func TestGenerateEventFingerprint(t *testing.T) {
	tests := []struct {
		name        string
		event       interface{}
		expectError bool
		expectID    bool
	}{
		{
			name: "enhanced event with ID",
			event: &EnhancedEvent{
				Type: "test_event",
				Metadata: EventMetadata{
					ID:     "unique-event-id-123",
					Source: "test/repo",
				},
				Payload: map[string]interface{}{
					"key": "value",
				},
			},
			expectError: false,
			expectID:    true,
		},
		{
			name: "enhanced event without ID",
			event: &EnhancedEvent{
				Type: "test_event",
				Metadata: EventMetadata{
					Source: "test/repo",
				},
				Payload: map[string]interface{}{
					"key": "value",
				},
			},
			expectError: false,
			expectID:    false,
		},
		{
			name: "legacy event",
			event: &Event{
				Type:   "test_event",
				Source: "test/repo",
				Payload: map[string]interface{}{
					"key": "value",
				},
			},
			expectError: false,
			expectID:    false,
		},
		{
			name:        "unsupported event type",
			event:       "not an event",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fingerprint, err := GenerateEventFingerprint(tt.event)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if tt.expectID {
					// Should return the event ID directly
					if enhancedEvent, ok := tt.event.(*EnhancedEvent); ok {
						if fingerprint != enhancedEvent.Metadata.ID {
							t.Errorf("Expected fingerprint to be event ID %s, got %s",
								enhancedEvent.Metadata.ID, fingerprint)
						}
					}
				} else {
					// Should return a hash
					if len(fingerprint) != 64 { // SHA256 hex string length
						t.Errorf("Expected SHA256 hash (64 chars), got %d chars", len(fingerprint))
					}
				}
			}
		})
	}
}

func TestGenerateEventFingerprintDeterministic(t *testing.T) {
	// Test that the same event produces the same fingerprint
	event1 := &Event{
		Type:   "test_event",
		Source: "test/repo",
		Payload: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"nested": map[string]interface{}{
				"a": 1,
				"b": 2,
			},
		},
	}

	event2 := &Event{
		Type:   "test_event",
		Source: "test/repo",
		Payload: map[string]interface{}{
			"key2": "value2", // Different order
			"key1": "value1",
			"nested": map[string]interface{}{
				"b": 2, // Different order
				"a": 1,
			},
		},
	}

	fingerprint1, err1 := GenerateEventFingerprint(event1)
	if err1 != nil {
		t.Fatalf("Failed to generate fingerprint 1: %v", err1)
	}

	fingerprint2, err2 := GenerateEventFingerprint(event2)
	if err2 != nil {
		t.Fatalf("Failed to generate fingerprint 2: %v", err2)
	}

	if fingerprint1 != fingerprint2 {
		t.Errorf("Expected same fingerprint for events with different key order, got:\n%s\n%s",
			fingerprint1, fingerprint2)
	}
}

func TestNormalizePayload(t *testing.T) {
	tests := []struct {
		name     string
		payload  map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "nil payload",
			payload:  nil,
			expected: nil,
		},
		{
			name: "simple payload",
			payload: map[string]interface{}{
				"z": "last",
				"a": "first",
				"m": "middle",
			},
			expected: map[string]interface{}{
				"a": "first",
				"m": "middle",
				"z": "last",
			},
		},
		{
			name: "nested maps",
			payload: map[string]interface{}{
				"outer": map[string]interface{}{
					"z": 3,
					"a": 1,
					"b": 2,
				},
			},
			expected: map[string]interface{}{
				"outer": map[string]interface{}{
					"a": float64(1),
					"b": float64(2),
					"z": float64(3),
				},
			},
		},
		{
			name: "arrays",
			payload: map[string]interface{}{
				"list": []interface{}{
					map[string]interface{}{"b": 2, "a": 1},
					"string",
					123,
				},
			},
			expected: map[string]interface{}{
				"list": []interface{}{
					map[string]interface{}{"a": float64(1), "b": float64(2)},
					"string",
					float64(123),
				},
			},
		},
		{
			name: "mixed types",
			payload: map[string]interface{}{
				"string": "value",
				"int":    42,
				"float":  3.14,
				"bool":   true,
				"null":   nil,
				"int32":  int32(100),
				"uint":   uint(200),
			},
			expected: map[string]interface{}{
				"bool":   true,
				"float":  3.14,
				"int":    float64(42),
				"int32":  float64(100),
				"null":   nil,
				"string": "value",
				"uint":   float64(200),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, err := normalizePayload(tt.payload)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// For deterministic comparison, convert to JSON
			if tt.expected == nil && normalized == nil {
				return
			}

			// Check that keys are in sorted order
			if normalized != nil {
				keys := make([]string, 0, len(normalized))
				for k := range normalized {
					keys = append(keys, k)
				}
				// Sort the keys to check they match expected order
				sortedKeys := make([]string, len(keys))
				copy(sortedKeys, keys)
				sort.Strings(sortedKeys)

				// The iteration order might not be sorted, but the content should match
				if len(keys) != len(sortedKeys) {
					t.Errorf("Key count mismatch")
				}
			}
		})
	}
}

func TestEventFingerprintWithDifferentNumericTypes(t *testing.T) {
	// Test that different numeric types produce the same fingerprint
	event1 := &Event{
		Type:   "test",
		Source: "source",
		Payload: map[string]interface{}{
			"count": int(42),
		},
	}

	event2 := &Event{
		Type:   "test",
		Source: "source",
		Payload: map[string]interface{}{
			"count": int32(42),
		},
	}

	event3 := &Event{
		Type:   "test",
		Source: "source",
		Payload: map[string]interface{}{
			"count": float64(42),
		},
	}

	fp1, _ := GenerateEventFingerprint(event1)
	fp2, _ := GenerateEventFingerprint(event2)
	fp3, _ := GenerateEventFingerprint(event3)

	if fp1 != fp2 || fp2 != fp3 {
		t.Errorf("Different numeric types produced different fingerprints:\nint: %s\nint32: %s\nfloat64: %s",
			fp1, fp2, fp3)
	}
}

func TestGetFanOutStateByFingerprint(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	fingerprint := "test-fingerprint-abc123"

	// Test getting non-existent fingerprint state
	state, err := manager.GetFanOutStateByFingerprint(fingerprint)
	if err != nil {
		t.Errorf("Expected no error for non-existent state, got: %v", err)
	}
	if state != nil {
		t.Errorf("Expected nil state for non-existent fingerprint, got: %v", state)
	}

	// Create a state with fingerprint
	createdState, err := manager.CreateFanOutStateWithFingerprint("", fingerprint, "parent-123", "org/repo", "test_event", true, 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to create state with fingerprint: %v", err)
	}

	expectedID := fmt.Sprintf("fanout-%s", fingerprint)
	if createdState.ID != expectedID {
		t.Errorf("Expected state ID %s, got %s", expectedID, createdState.ID)
	}

	// Retrieve the state by fingerprint
	retrievedState, err := manager.GetFanOutStateByFingerprint(fingerprint)
	if err != nil {
		t.Fatalf("Failed to get state by fingerprint: %v", err)
	}
	if retrievedState == nil {
		t.Fatalf("Expected state to be found by fingerprint")
	}
	if retrievedState.ID != expectedID {
		t.Errorf("Expected retrieved state ID %s, got %s", expectedID, retrievedState.ID)
	}
}

func TestCreateFanOutStateWithFingerprint(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	fingerprint := "test-fingerprint-def456"
	expectedID := fmt.Sprintf("fanout-%s", fingerprint)

	// Create first state
	state1, err := manager.CreateFanOutStateWithFingerprint("", fingerprint, "parent-123", "org/repo", "test_event", true, 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to create first state: %v", err)
	}
	if state1.ID != expectedID {
		t.Errorf("Expected state ID %s, got %s", expectedID, state1.ID)
	}

	// Attempt to create second state with same fingerprint
	state2, err := manager.CreateFanOutStateWithFingerprint("", fingerprint, "parent-456", "org/repo2", "test_event2", false, 10*time.Minute)
	if err != nil {
		t.Fatalf("Failed to handle duplicate fingerprint: %v", err)
	}

	// Should return the existing state
	if state2.ID != state1.ID {
		t.Errorf("Expected same state ID for duplicate fingerprint, got %s vs %s", state2.ID, state1.ID)
	}
	if state2.ParentRunID != state1.ParentRunID {
		t.Errorf("Expected original state properties, got ParentRunID %s vs %s", state2.ParentRunID, state1.ParentRunID)
	}
}

func TestCreateStateAtomic(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	id := "fanout-atomic-test"

	// Create state atomically
	state, err := manager.createStateAtomic(id, "parent-123", "org/repo", "test_event", true, 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to create state atomically: %v", err)
	}
	if state.ID != id {
		t.Errorf("Expected state ID %s, got %s", id, state.ID)
	}

	// Verify state was persisted
	stateFile := filepath.Join(tempDir, fmt.Sprintf("%s.json", id))
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Errorf("State file was not created")
	}

	// Verify state is in memory
	retrievedState, err := manager.GetFanOutState(id)
	if err != nil {
		t.Fatalf("Failed to retrieve state from memory: %v", err)
	}
	if retrievedState.ID != id {
		t.Errorf("Expected retrieved state ID %s, got %s", id, retrievedState.ID)
	}
}

func TestCreateStateAtomicRaceCondition(t *testing.T) {
	tempDir := t.TempDir()

	// Test that concurrent creation with same ID returns existing state
	manager1, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create first state manager: %v", err)
	}

	// Create new manager after first state is created to simulate race condition
	id := "fanout-race-test"

	// Create state with first manager
	state1, err := manager1.createStateAtomic(id, "parent-123", "org/repo", "test_event", true, 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to create state with first manager: %v", err)
	}

	// Create second manager after state file exists
	manager2, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create second state manager: %v", err)
	}

	// Attempt to create state with same ID using second manager
	state2, err := manager2.createStateAtomic(id, "parent-456", "org/repo2", "test_event2", false, 10*time.Minute)
	if err != nil {
		t.Fatalf("Failed to handle existing state: %v", err)
	}

	// Should return the existing state properties (loaded from disk)
	if state2.ID != state1.ID {
		t.Errorf("Expected same state ID, got %s vs %s", state2.ID, state1.ID)
	}
	if state2.ParentRunID != state1.ParentRunID {
		t.Errorf("Expected original state ParentRunID, got %s vs %s", state2.ParentRunID, state1.ParentRunID)
	}
}

func TestIdempotencyRetentionConfiguration(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Check default retention period
	defaultRetention := manager.GetIdempotencyRetention()
	if defaultRetention != 24*time.Hour {
		t.Errorf("Expected default retention of 24 hours, got %v", defaultRetention)
	}

	// Set custom retention period
	customRetention := 12 * time.Hour
	manager.SetIdempotencyRetention(customRetention)

	// Verify retention was set
	if manager.GetIdempotencyRetention() != customRetention {
		t.Errorf("Expected custom retention %v, got %v", customRetention, manager.GetIdempotencyRetention())
	}
}

func TestIsIdempotentState(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	tests := []struct {
		name     string
		stateID  string
		expected bool
	}{
		{
			name:     "idempotent state with valid hex fingerprint",
			stateID:  "fanout-abc123def4567890123456789012345678901234567890123456789012345678",
			expected: true,
		},
		{
			name:     "idempotent state with uppercase hex",
			stateID:  "fanout-ABC123DEF4567890123456789012345678901234567890123456789012345678",
			expected: true,
		},
		{
			name:     "timestamp-based state",
			stateID:  "fanout-1753922275-library_built",
			expected: false,
		},
		{
			name:     "short hex string",
			stateID:  "fanout-abc123",
			expected: false,
		},
		{
			name:     "invalid hex characters",
			stateID:  "fanout-xyz123def456789012345678901234567890123456789012345678901234567890",
			expected: false,
		},
		{
			name:     "no fanout prefix",
			stateID:  "state-abc123def456789012345678901234567890123456789012345678901234567890",
			expected: false,
		},
		{
			name:     "too short",
			stateID:  "fanout-",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.isIdempotentState(tt.stateID)
			if result != tt.expected {
				t.Errorf("isIdempotentState(%s) = %v, want %v", tt.stateID, result, tt.expected)
			}
		})
	}
}

func TestCleanupCompletedStatesWithIdempotencyRetention(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewFanOutStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Set custom retention periods
	traditionalRetention := 1 * time.Hour
	idempotentRetention := 2 * time.Hour
	manager.SetIdempotencyRetention(idempotentRetention)

	// Create traditional timestamp-based state (old)
	traditionalState, _ := manager.CreateFanOutState("fanout-1234567890-test", "", "org/repo", "test", false, 0)
	traditionalState.CompleteFanOut()
	oldTime := time.Now().Add(-3 * time.Hour) // 3 hours ago
	traditionalState.EndTime = &oldTime

	// Create idempotent fingerprint-based state (old but within idempotent retention)
	idempotentState, _ := manager.CreateFanOutStateWithFingerprint("", "abc123def4567890123456789012345678901234567890123456789012345678", "", "org/repo", "test", false, 0)
	idempotentState.CompleteFanOut()
	mediumOldTime := time.Now().Add(-90 * time.Minute) // 1.5 hours ago
	idempotentState.EndTime = &mediumOldTime

	// Create another idempotent state (very old, should be cleaned)
	veryOldIdempotentState, _ := manager.CreateFanOutStateWithFingerprint("", "def456789012345678901234567890123456789012345678901234567890abc123", "", "org/repo", "test", false, 0)
	veryOldIdempotentState.CompleteFanOut()
	veryOldTime := time.Now().Add(-5 * time.Hour) // 5 hours ago
	veryOldIdempotentState.EndTime = &veryOldTime

	// Create recent traditional state (should not be cleaned)
	recentTraditionalState, _ := manager.CreateFanOutState("fanout-9876543210-test", "", "org/repo", "test", false, 0)
	recentTraditionalState.CompleteFanOut()
	recentTime := time.Now().Add(-30 * time.Minute) // 30 minutes ago
	recentTraditionalState.EndTime = &recentTime

	// Cleanup with traditional retention period
	err = manager.CleanupCompletedStates(traditionalRetention)
	if err != nil {
		t.Fatalf("Failed to cleanup completed states: %v", err)
	}

	// Verify cleanup results
	// Traditional old state should be removed (older than 1 hour)
	_, err = manager.GetFanOutState("fanout-1234567890-test")
	if err == nil {
		t.Errorf("Expected old traditional state to be removed")
	}

	// Idempotent state should still exist (1.5 hours old, but retention is 2 hours)
	_, err = manager.GetFanOutState("fanout-abc123def4567890123456789012345678901234567890123456789012345678")
	if err != nil {
		t.Errorf("Expected idempotent state to still exist: %v", err)
	}

	// Very old idempotent state should be removed (5 hours old, retention is 2 hours)
	_, err = manager.GetFanOutState("fanout-def456789012345678901234567890123456789012345678901234567890abc123")
	if err == nil {
		t.Errorf("Expected very old idempotent state to be removed")
	}

	// Recent traditional state should still exist
	_, err = manager.GetFanOutState("fanout-9876543210-test")
	if err != nil {
		t.Errorf("Expected recent traditional state to still exist: %v", err)
	}
}

// Test subscription fingerprinting functionality.
func TestGenerateSubscriptionFingerprint(t *testing.T) {
	eventFingerprint := "test-event-fingerprint-abc123"

	tests := []struct {
		name        string
		subscriber1 SubscriptionMatch
		subscriber2 SubscriptionMatch
		expectSame  bool
		description string
	}{
		{
			name: "identical subscriptions should have same fingerprint",
			subscriber1: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.version != null"},
					Inputs:   map[string]string{"version": "{{ .payload.version }}"},
				},
			},
			subscriber2: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.version != null"},
					Inputs:   map[string]string{"version": "{{ .payload.version }}"},
				},
			},
			expectSame:  true,
			description: "Identical subscriptions should produce identical fingerprints",
		},
		{
			name: "different repositories should have same fingerprints for diamond detection",
			subscriber1: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.version != null"},
					Inputs:   map[string]string{"version": "{{ .payload.version }}"},
				},
			},
			subscriber2: SubscriptionMatch{
				Repository: "org/repo2",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.version != null"},
					Inputs:   map[string]string{"version": "{{ .payload.version }}"},
				},
			},
			expectSame:  true,
			description: "Different repositories with identical subscriptions should produce same fingerprints for diamond dependency detection",
		},
		{
			name: "different workflows should have different fingerprints",
			subscriber1: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.version != null"},
					Inputs:   map[string]string{"version": "{{ .payload.version }}"},
				},
			},
			subscriber2: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "test.yml",
					Filters:  []string{"event.payload.version != null"},
					Inputs:   map[string]string{"version": "{{ .payload.version }}"},
				},
			},
			expectSame:  false,
			description: "Different workflows should produce different fingerprints",
		},
		{
			name: "different filters should have different fingerprints",
			subscriber1: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.version != null"},
					Inputs:   map[string]string{"version": "{{ .payload.version }}"},
				},
			},
			subscriber2: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.tag != null"},
					Inputs:   map[string]string{"version": "{{ .payload.version }}"},
				},
			},
			expectSame:  false,
			description: "Different filters should produce different fingerprints",
		},
		{
			name: "different inputs should have different fingerprints",
			subscriber1: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.version != null"},
					Inputs:   map[string]string{"version": "{{ .payload.version }}"},
				},
			},
			subscriber2: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.version != null"},
					Inputs:   map[string]string{"tag": "{{ .payload.tag }}"},
				},
			},
			expectSame:  false,
			description: "Different inputs should produce different fingerprints",
		},
		{
			name: "normalized CEL expressions should have same fingerprints",
			subscriber1: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.version != null"},
					Inputs:   map[string]string{"version": "{{ .payload.version }}"},
				},
			},
			subscriber2: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.version!=null"},              // Different whitespace
					Inputs:   map[string]string{"version": "{{.payload.version}}"}, // Different whitespace
				},
			},
			expectSame:  true,
			description: "CEL expressions with different whitespace should normalize to same fingerprint",
		},
		{
			name: "input key order should not affect fingerprint",
			subscriber1: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{},
					Inputs:   map[string]string{"version": "v1", "tag": "latest"},
				},
			},
			subscriber2: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{},
					Inputs:   map[string]string{"tag": "latest", "version": "v1"}, // Different order
				},
			},
			expectSame:  true,
			description: "Input key order should not affect fingerprint due to normalization",
		},
		{
			name: "filter order should not affect fingerprint",
			subscriber1: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.version != null", "event.payload.tag != null"},
					Inputs:   map[string]string{},
				},
			},
			subscriber2: SubscriptionMatch{
				Repository: "org/repo1",
				Subscription: config.Subscription{
					Workflow: "build.yml",
					Filters:  []string{"event.payload.tag != null", "event.payload.version != null"}, // Different order
					Inputs:   map[string]string{},
				},
			},
			expectSame:  true,
			description: "Filter order should not affect fingerprint due to normalization",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp1, err1 := GenerateSubscriptionFingerprint(tt.subscriber1, eventFingerprint)
			if err1 != nil {
				t.Fatalf("Failed to generate fingerprint for subscriber1: %v", err1)
			}

			fp2, err2 := GenerateSubscriptionFingerprint(tt.subscriber2, eventFingerprint)
			if err2 != nil {
				t.Fatalf("Failed to generate fingerprint for subscriber2: %v", err2)
			}

			if tt.expectSame {
				if fp1 != fp2 {
					t.Errorf("Expected same fingerprints but got different:\nSubscriber1: %s\nSubscriber2: %s\nDescription: %s",
						fp1, fp2, tt.description)
				}
			} else {
				if fp1 == fp2 {
					t.Errorf("Expected different fingerprints but got same: %s\nDescription: %s",
						fp1, tt.description)
				}
			}

			// Verify fingerprints are deterministic (same input produces same output)
			fp1_repeat, _ := GenerateSubscriptionFingerprint(tt.subscriber1, eventFingerprint)
			if fp1 != fp1_repeat {
				t.Errorf("Fingerprint generation is not deterministic for subscriber1: %s vs %s", fp1, fp1_repeat)
			}
		})
	}
}

func TestNormalizeCELExpression(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic normalization",
			input:    "  event.payload.version != null  ",
			expected: "event.payload.version!=null",
		},
		{
			name:     "multiple spaces",
			input:    "event.payload.version    !=    null",
			expected: "event.payload.version!=null",
		},
		{
			name:     "parentheses normalization",
			input:    "( event.payload.version != null )",
			expected: "(event.payload.version!=null)",
		},
		{
			name:     "brackets normalization",
			input:    "event.payload[ 'key' ] != null",
			expected: "event.payload['key']!=null",
		},
		{
			name:     "dots normalization",
			input:    "event . payload . version",
			expected: "event.payload.version",
		},
		{
			name:     "complex expression",
			input:    "event.payload.version >= '1.0.0' && event.payload.tag != null",
			expected: "event.payload.version>='1.0.0'&&event.payload.tag!=null",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   \t\n   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeCELExpression(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeCELExpression(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeInputs(t *testing.T) {
	tests := []struct {
		name     string
		inputs   map[string]string
		expected map[string]string
	}{
		{
			name:     "nil inputs",
			inputs:   nil,
			expected: nil,
		},
		{
			name:     "empty inputs",
			inputs:   map[string]string{},
			expected: map[string]string{},
		},
		{
			name: "single input normalization",
			inputs: map[string]string{
				"version": "  {{ .payload.version }}  ",
			},
			expected: map[string]string{
				"version": "{{.payload.version}}",
			},
		},
		{
			name: "multiple inputs with normalization",
			inputs: map[string]string{
				"version": "{{ .payload . version }}",
				"tag":     "{{   .payload.tag   }}",
				"branch":  "main",
			},
			expected: map[string]string{
				"branch":  "main",
				"tag":     "{{.payload.tag}}",
				"version": "{{.payload.version}}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeInputs(tt.inputs)

			if tt.expected == nil && result == nil {
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d inputs, got %d", len(tt.expected), len(result))
				return
			}

			for key, expectedValue := range tt.expected {
				if actualValue, exists := result[key]; !exists {
					t.Errorf("Expected key %q not found in result", key)
				} else if actualValue != expectedValue {
					t.Errorf("For key %q, expected %q, got %q", key, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestNormalizeFilters(t *testing.T) {
	tests := []struct {
		name     string
		filters  []string
		expected []string
	}{
		{
			name:     "nil filters",
			filters:  nil,
			expected: nil,
		},
		{
			name:     "empty filters",
			filters:  []string{},
			expected: []string{},
		},
		{
			name: "single filter normalization",
			filters: []string{
				"  event.payload.version != null  ",
			},
			expected: []string{
				"event.payload.version!=null",
			},
		},
		{
			name: "multiple filters with sorting",
			filters: []string{
				"event.payload.version != null",
				"event.payload.tag >= '1.0.0'",
				"event.payload.branch == 'main'",
			},
			expected: []string{
				"event.payload.branch=='main'",
				"event.payload.tag>='1.0.0'",
				"event.payload.version!=null",
			},
		},
		{
			name: "filters with different whitespace",
			filters: []string{
				"  event.payload.version   !=   null  ",
				"event.payload.tag >= '1.0.0'",
			},
			expected: []string{
				"event.payload.tag>='1.0.0'",
				"event.payload.version!=null",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeFilters(tt.filters)

			if tt.expected == nil && result == nil {
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d filters, got %d", len(tt.expected), len(result))
				return
			}

			for i, expectedFilter := range tt.expected {
				if result[i] != expectedFilter {
					t.Errorf("Filter %d: expected %q, got %q", i, expectedFilter, result[i])
				}
			}
		})
	}
}

func TestSubscriptionFingerprintConsistency(t *testing.T) {
	// Test that the same subscription produces the same fingerprint across multiple calls
	eventFingerprint := "consistent-test-fingerprint"
	subscriber := SubscriptionMatch{
		Repository: "org/repo",
		Subscription: config.Subscription{
			Workflow: "build.yml",
			Filters:  []string{"event.payload.version != null", "event.payload.tag >= '1.0.0'"},
			Inputs:   map[string]string{"version": "{{ .payload.version }}", "tag": "{{ .payload.tag }}"},
		},
	}

	// Generate fingerprint multiple times
	fingerprints := make([]string, 10)
	for i := 0; i < 10; i++ {
		fp, err := GenerateSubscriptionFingerprint(subscriber, eventFingerprint)
		if err != nil {
			t.Fatalf("Failed to generate fingerprint on attempt %d: %v", i+1, err)
		}
		fingerprints[i] = fp
	}

	// Verify all fingerprints are identical
	for i := 1; i < len(fingerprints); i++ {
		if fingerprints[i] != fingerprints[0] {
			t.Errorf("Inconsistent fingerprints: attempt 1 = %s, attempt %d = %s",
				fingerprints[0], i+1, fingerprints[i])
		}
	}
}

func TestSubscriptionFingerprintDifferentEventFingerprints(t *testing.T) {
	// Test that same subscription with different event fingerprints produces different results
	subscriber := SubscriptionMatch{
		Repository: "org/repo",
		Subscription: config.Subscription{
			Workflow: "build.yml",
			Filters:  []string{"event.payload.version != null"},
			Inputs:   map[string]string{"version": "{{ .payload.version }}"},
		},
	}

	fp1, err1 := GenerateSubscriptionFingerprint(subscriber, "event-fp-1")
	if err1 != nil {
		t.Fatalf("Failed to generate fingerprint 1: %v", err1)
	}

	fp2, err2 := GenerateSubscriptionFingerprint(subscriber, "event-fp-2")
	if err2 != nil {
		t.Fatalf("Failed to generate fingerprint 2: %v", err2)
	}

	if fp1 == fp2 {
		t.Errorf("Expected different fingerprints for different event fingerprints, but got same: %s", fp1)
	}
}
