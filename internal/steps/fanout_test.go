package steps

import (
	"context"
	"testing"

	"github.com/dangazineu/tako/internal/interfaces"
)

// mockSubscriptionDiscoverer implements the SubscriptionDiscoverer interface for testing.
type mockSubscriptionDiscoverer struct {
	findSubscribersFunc func(artifact, eventType string) ([]interfaces.SubscriptionMatch, error)
}

func (m *mockSubscriptionDiscoverer) FindSubscribers(artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
	if m.findSubscribersFunc != nil {
		return m.findSubscribersFunc(artifact, eventType)
	}
	return []interfaces.SubscriptionMatch{}, nil
}

// mockWorkflowRunner implements the WorkflowRunner interface for testing.
type mockWorkflowRunner struct {
	executeWorkflowFunc func(ctx context.Context, repoPath, workflowName string, inputs map[string]string) (*interfaces.ExecutionResult, error)
}

func (m *mockWorkflowRunner) ExecuteWorkflow(ctx context.Context, repoPath, workflowName string, inputs map[string]string) (*interfaces.ExecutionResult, error) {
	if m.executeWorkflowFunc != nil {
		return m.executeWorkflowFunc(ctx, repoPath, workflowName, inputs)
	}
	return &interfaces.ExecutionResult{
		Success: true,
	}, nil
}

func TestNewFanOutStepExecutor(t *testing.T) {
	discoverer := &mockSubscriptionDiscoverer{}
	runner := &mockWorkflowRunner{}

	executor := NewFanOutStepExecutor(discoverer, runner)

	if executor == nil {
		t.Fatal("Expected non-nil executor")
	}

	if executor.discoverer != discoverer {
		t.Error("Expected discoverer to be set correctly")
	}

	if executor.runner != runner {
		t.Error("Expected runner to be set correctly")
	}
}

func TestFanOutStepExecutor_Dependencies(t *testing.T) {
	discoverer := &mockSubscriptionDiscoverer{}
	runner := &mockWorkflowRunner{}

	executor := NewFanOutStepExecutor(discoverer, runner)

	// Verify that the executor holds references to the provided dependencies
	if executor.discoverer == nil {
		t.Error("Expected discoverer to be non-nil")
	}

	if executor.runner == nil {
		t.Error("Expected runner to be non-nil")
	}
}

func TestFanOutStepParams_Structure(t *testing.T) {
	params := FanOutStepParams{
		EventType:        "test_event",
		WaitForChildren:  true,
		Timeout:          "30s",
		ConcurrencyLimit: 5,
		Payload:          map[string]interface{}{"key": "value"},
		SchemaVersion:    "1.0.0",
	}

	if params.EventType != "test_event" {
		t.Error("Expected EventType to be set correctly")
	}

	if !params.WaitForChildren {
		t.Error("Expected WaitForChildren to be true")
	}

	if params.Timeout != "30s" {
		t.Error("Expected Timeout to be set correctly")
	}

	if params.ConcurrencyLimit != 5 {
		t.Error("Expected ConcurrencyLimit to be set correctly")
	}

	if params.Payload["key"] != "value" {
		t.Error("Expected Payload to be set correctly")
	}

	if params.SchemaVersion != "1.0.0" {
		t.Error("Expected SchemaVersion to be set correctly")
	}
}

func TestFanOutStepResult_Structure(t *testing.T) {
	result := FanOutStepResult{
		Success:          true,
		EventEmitted:     true,
		SubscribersFound: 3,
		TriggeredCount:   2,
		Errors:           []string{"error1", "error2"},
		FanOutID:         "test-fanout-123",
	}

	if !result.Success {
		t.Error("Expected Success to be true")
	}

	if !result.EventEmitted {
		t.Error("Expected EventEmitted to be true")
	}

	if result.SubscribersFound != 3 {
		t.Error("Expected SubscribersFound to be 3")
	}

	if result.TriggeredCount != 2 {
		t.Error("Expected TriggeredCount to be 2")
	}

	if len(result.Errors) != 2 {
		t.Error("Expected 2 errors")
	}

	if result.FanOutID != "test-fanout-123" {
		t.Error("Expected FanOutID to be set correctly")
	}
}
