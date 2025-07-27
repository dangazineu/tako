package steps

import (
	"context"
	"fmt"
	"testing"

	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/interfaces"
)

// MockWorkflowRunner implements WorkflowRunner for testing.
type MockWorkflowRunner struct {
	executions []MockExecution
	shouldFail bool
}

type MockExecution struct {
	RepoPath     string
	WorkflowName string
	Inputs       map[string]string
	RunID        string
}

func (m *MockWorkflowRunner) ExecuteChildWorkflow(ctx context.Context, repoPath, workflowName string, inputs map[string]string) (string, error) {
	if m.shouldFail {
		return "", fmt.Errorf("mock execution failed")
	}

	runID := fmt.Sprintf("run-%d", len(m.executions)+1)
	m.executions = append(m.executions, MockExecution{
		RepoPath:     repoPath,
		WorkflowName: workflowName,
		Inputs:       inputs,
		RunID:        runID,
	})

	return runID, nil
}

// MockOrchestrator implements SubscriptionDiscoverer for testing.
type MockOrchestrator struct {
	subscriptionMatches []interfaces.SubscriptionMatch
	shouldFail          bool
}

// MockOrchestrator implements SubscriptionDiscoverer interface.
var _ interfaces.SubscriptionDiscoverer = (*MockOrchestrator)(nil)

func (m *MockOrchestrator) DiscoverSubscriptions(ctx context.Context, eventType, artifactRef string, eventPayload map[string]string) ([]interfaces.SubscriptionMatch, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mock discovery failed")
	}
	return m.subscriptionMatches, nil
}

func TestNewFanOutExecutor(t *testing.T) {
	mockOrchestrator := &MockOrchestrator{}
	mockRunner := &MockWorkflowRunner{}

	executor := NewFanOutExecutor(mockOrchestrator, mockRunner)

	if executor.orchestrator != mockOrchestrator {
		t.Error("Expected orchestrator to be set correctly")
	}
	if executor.runner != mockRunner {
		t.Error("Expected runner to be set correctly")
	}
}

func TestParseParameters(t *testing.T) {
	executor := &FanOutExecutor{}

	tests := []struct {
		name        string
		step        config.WorkflowStep
		expected    *FanOutStepParams
		expectError bool
	}{
		{
			name: "minimal valid parameters",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type": "library_built",
				},
			},
			expected: &FanOutStepParams{
				EventType:        "library_built",
				WaitForChildren:  false,
				ConcurrencyLimit: 0,
			},
			expectError: false,
		},
		{
			name: "all parameters",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type":        "library_built",
					"wait_for_children": true,
					"timeout":           "2h",
					"concurrency_limit": 4,
				},
			},
			expected: &FanOutStepParams{
				EventType:        "library_built",
				WaitForChildren:  true,
				Timeout:          "2h",
				ConcurrencyLimit: 4,
			},
			expectError: false,
		},
		{
			name: "missing event_type",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"wait_for_children": true,
				},
			},
			expectError: true,
		},
		{
			name: "invalid event_type type",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type": 123,
				},
			},
			expectError: true,
		},
		{
			name: "invalid wait_for_children type",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type":        "library_built",
					"wait_for_children": "yes",
				},
			},
			expectError: true,
		},
		{
			name: "invalid timeout type",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type": "library_built",
					"timeout":    123,
				},
			},
			expectError: true,
		},
		{
			name: "concurrency_limit as float",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type":        "library_built",
					"concurrency_limit": 4.0,
				},
			},
			expected: &FanOutStepParams{
				EventType:        "library_built",
				WaitForChildren:  false,
				ConcurrencyLimit: 4,
			},
			expectError: false,
		},
		{
			name: "concurrency_limit as string",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type":        "library_built",
					"concurrency_limit": "8",
				},
			},
			expected: &FanOutStepParams{
				EventType:        "library_built",
				WaitForChildren:  false,
				ConcurrencyLimit: 8,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.parseParameters(tt.step)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.EventType != tt.expected.EventType {
				t.Errorf("Expected EventType %s, got %s", tt.expected.EventType, result.EventType)
			}
			if result.WaitForChildren != tt.expected.WaitForChildren {
				t.Errorf("Expected WaitForChildren %t, got %t", tt.expected.WaitForChildren, result.WaitForChildren)
			}
			if result.Timeout != tt.expected.Timeout {
				t.Errorf("Expected Timeout %s, got %s", tt.expected.Timeout, result.Timeout)
			}
			if result.ConcurrencyLimit != tt.expected.ConcurrencyLimit {
				t.Errorf("Expected ConcurrencyLimit %d, got %d", tt.expected.ConcurrencyLimit, result.ConcurrencyLimit)
			}
		})
	}
}

func TestValidateParameters(t *testing.T) {
	executor := &FanOutExecutor{}

	tests := []struct {
		name        string
		params      *FanOutStepParams
		expectError bool
	}{
		{
			name: "valid minimal parameters",
			params: &FanOutStepParams{
				EventType: "library_built",
			},
			expectError: false,
		},
		{
			name: "valid timeout",
			params: &FanOutStepParams{
				EventType: "library_built",
				Timeout:   "2h30m",
			},
			expectError: false,
		},
		{
			name: "empty event type",
			params: &FanOutStepParams{
				EventType: "",
			},
			expectError: true,
		},
		{
			name: "invalid timeout format",
			params: &FanOutStepParams{
				EventType: "library_built",
				Timeout:   "invalid",
			},
			expectError: true,
		},
		{
			name: "negative concurrency limit",
			params: &FanOutStepParams{
				EventType:        "library_built",
				ConcurrencyLimit: -1,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.validateParameters(tt.params)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestMapEventToInputs(t *testing.T) {
	executor := &FanOutExecutor{}

	tests := []struct {
		name           string
		inputMappings  map[string]string
		eventPayload   map[string]string
		expectedInputs map[string]string
	}{
		{
			name: "direct mapping",
			inputMappings: map[string]string{
				"version":  "version",
				"artifact": "artifact_name",
			},
			eventPayload: map[string]string{
				"version":       "1.2.3",
				"artifact_name": "my-library",
			},
			expectedInputs: map[string]string{
				"version":  "1.2.3",
				"artifact": "my-library",
			},
		},
		{
			name: "missing payload values",
			inputMappings: map[string]string{
				"version":     "version",
				"environment": "prod",
			},
			eventPayload: map[string]string{
				"version": "1.2.3",
			},
			expectedInputs: map[string]string{
				"version":     "1.2.3",
				"environment": "prod", // Uses literal value when not in payload
			},
		},
		{
			name:           "empty mappings",
			inputMappings:  map[string]string{},
			eventPayload:   map[string]string{"version": "1.2.3"},
			expectedInputs: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.mapEventToInputs(tt.inputMappings, tt.eventPayload)

			if len(result) != len(tt.expectedInputs) {
				t.Errorf("Expected %d inputs, got %d", len(tt.expectedInputs), len(result))
			}

			for key, expectedValue := range tt.expectedInputs {
				if actualValue, exists := result[key]; !exists {
					t.Errorf("Expected input %s not found", key)
				} else if actualValue != expectedValue {
					t.Errorf("Expected input %s = %s, got %s", key, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestExecute(t *testing.T) {
	tests := []struct {
		name                string
		step                config.WorkflowStep
		artifactRef         string
		eventPayload        map[string]string
		subscriptionMatches []interfaces.SubscriptionMatch
		expectedTriggered   int
		expectError         bool
	}{
		{
			name: "successful fan-out",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type": "library_built",
				},
			},
			artifactRef:  "owner/repo:library",
			eventPayload: map[string]string{"version": "1.2.3"},
			subscriptionMatches: []interfaces.SubscriptionMatch{
				{
					RepositoryName: "org/app1",
					RepositoryPath: "/cache/repos/org/app1/main",
					Subscription: config.Subscription{
						Workflow: "update-deps",
						Inputs: map[string]string{
							"lib_version": "version",
						},
					},
				},
				{
					RepositoryName: "org/app2",
					RepositoryPath: "/cache/repos/org/app2/main",
					Subscription: config.Subscription{
						Workflow: "integration-test",
						Inputs:   map[string]string{},
					},
				},
			},
			expectedTriggered: 2,
			expectError:       false,
		},
		{
			name: "no matching subscriptions",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type": "library_built",
				},
			},
			artifactRef:         "owner/repo:library",
			eventPayload:        map[string]string{"version": "1.2.3"},
			subscriptionMatches: []interfaces.SubscriptionMatch{},
			expectedTriggered:   0,
			expectError:         false,
		},
		{
			name: "invalid step parameters",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"invalid_param": "value",
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockOrchestrator := &MockOrchestrator{
				subscriptionMatches: tt.subscriptionMatches,
			}
			mockRunner := &MockWorkflowRunner{}

			executor := NewFanOutExecutor(mockOrchestrator, mockRunner)
			ctx := context.Background()

			result, err := executor.Execute(ctx, tt.step, tt.artifactRef, tt.eventPayload)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !result.EventEmitted {
				t.Error("Expected event to be marked as emitted")
			}

			if len(result.TriggeredWorkflows) != tt.expectedTriggered {
				t.Errorf("Expected %d triggered workflows, got %d", tt.expectedTriggered, len(result.TriggeredWorkflows))
			}

			if len(result.ChildRunIDs) != tt.expectedTriggered {
				t.Errorf("Expected %d child run IDs, got %d", tt.expectedTriggered, len(result.ChildRunIDs))
			}

			// Verify mock runner was called correctly
			if len(mockRunner.executions) != tt.expectedTriggered {
				t.Errorf("Expected %d workflow executions, got %d", tt.expectedTriggered, len(mockRunner.executions))
			}

			// Verify workflow executions match expectations
			for i, execution := range mockRunner.executions {
				if i >= len(tt.subscriptionMatches) {
					break
				}

				expectedMatch := tt.subscriptionMatches[i]
				if execution.RepoPath != expectedMatch.RepositoryPath {
					t.Errorf("Expected execution %d repo path %s, got %s", i, expectedMatch.RepositoryPath, execution.RepoPath)
				}
				if execution.WorkflowName != expectedMatch.Subscription.Workflow {
					t.Errorf("Expected execution %d workflow %s, got %s", i, expectedMatch.Subscription.Workflow, execution.WorkflowName)
				}

				// Verify input mapping was applied
				for inputName, inputMapping := range expectedMatch.Subscription.Inputs {
					expectedValue := tt.eventPayload[inputMapping]
					if expectedValue == "" {
						expectedValue = inputMapping // Literal value
					}
					if execution.Inputs[inputName] != expectedValue {
						t.Errorf("Expected execution %d input %s = %s, got %s", i, inputName, expectedValue, execution.Inputs[inputName])
					}
				}
			}
		})
	}
}

func TestExecute_WithFailures(t *testing.T) {
	// Test handling of workflow execution failures
	mockOrchestrator := &MockOrchestrator{
		subscriptionMatches: []interfaces.SubscriptionMatch{
			{
				RepositoryName: "org/app1",
				RepositoryPath: "/cache/repos/org/app1/main",
				Subscription: config.Subscription{
					Workflow: "update-deps",
				},
			},
		},
	}

	// Configure runner to fail
	mockRunner := &MockWorkflowRunner{shouldFail: true}

	executor := NewFanOutExecutor(mockOrchestrator, mockRunner)
	ctx := context.Background()

	step := config.WorkflowStep{
		Uses: "tako/fan-out@v1",
		With: map[string]interface{}{
			"event_type": "library_built",
		},
	}

	result, err := executor.Execute(ctx, step, "owner/repo:library", map[string]string{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should continue despite failures
	if !result.EventEmitted {
		t.Error("Expected event to be marked as emitted")
	}

	if len(result.TriggeredWorkflows) != 1 {
		t.Errorf("Expected 1 triggered workflow, got %d", len(result.TriggeredWorkflows))
	}

	if len(result.TriggeredWorkflows) > 0 && result.TriggeredWorkflows[0].Status != "failed" {
		t.Errorf("Expected failed status, got %s", result.TriggeredWorkflows[0].Status)
	}

	// No child run IDs should be recorded for failed executions
	if len(result.ChildRunIDs) != 0 {
		t.Errorf("Expected 0 child run IDs for failed executions, got %d", len(result.ChildRunIDs))
	}
}

func TestExecute_OrchestratorFailure(t *testing.T) {
	// Test handling of orchestrator discovery failures
	mockOrchestrator := &MockOrchestrator{shouldFail: true}
	mockRunner := &MockWorkflowRunner{}

	executor := NewFanOutExecutor(mockOrchestrator, mockRunner)
	ctx := context.Background()

	step := config.WorkflowStep{
		Uses: "tako/fan-out@v1",
		With: map[string]interface{}{
			"event_type": "library_built",
		},
	}

	_, err := executor.Execute(ctx, step, "owner/repo:library", map[string]string{})
	if err == nil {
		t.Error("Expected error from orchestrator failure")
	}
}
