package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/interfaces"
)

// MockDiscoverer implements the SubscriptionDiscoverer interface for testing.
type MockDiscoverer struct {
	subscriptions []interfaces.SubscriptionMatch
	err           error
	called        bool
	artifact      string
	eventType     string
}

func (m *MockDiscoverer) FindSubscribers(artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
	m.called = true
	m.artifact = artifact
	m.eventType = eventType
	return m.subscriptions, m.err
}

func TestExecuteBuiltinStep_FanOut(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Create test subscriptions
	testSubscriptions := []interfaces.SubscriptionMatch{
		{
			Repository: "test-org/consumer1",
			Subscription: config.Subscription{
				Artifact: "test-org/producer:library",
				Events:   []string{"build_completed"},
				Workflow: "update",
			},
			RepoPath: filepath.Join(tempDir, "repos/test-org/consumer1"),
		},
		{
			Repository: "test-org/consumer2",
			Subscription: config.Subscription{
				Artifact: "test-org/producer:library",
				Events:   []string{"build_completed"},
				Workflow: "deploy",
			},
			RepoPath: filepath.Join(tempDir, "repos/test-org/consumer2"),
		},
	}

	tests := []struct {
		name              string
		step              config.WorkflowStep
		mockSubscriptions []interfaces.SubscriptionMatch
		mockErr           error
		expectError       bool
		expectSuccess     bool
	}{
		{
			name: "successful fan-out with discovered subscriptions",
			step: config.WorkflowStep{
				ID:   "fan-out-step",
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type": "build_completed",
					"payload": map[string]interface{}{
						"version": "1.0.0",
					},
				},
			},
			mockSubscriptions: testSubscriptions,
			expectSuccess:     true,
		},
		{
			name: "fan-out with no subscriptions",
			step: config.WorkflowStep{
				ID:   "fan-out-step",
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type": "unknown_event",
				},
			},
			mockSubscriptions: []interfaces.SubscriptionMatch{},
			expectSuccess:     true,
		},
		{
			name: "missing event_type parameter",
			step: config.WorkflowStep{
				ID:   "fan-out-step",
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"payload": map[string]interface{}{
						"version": "1.0.0",
					},
				},
			},
			expectError: true,
		},
		{
			name: "unknown built-in step",
			step: config.WorkflowStep{
				ID:   "unknown-step",
				Uses: "tako/unknown@v1",
				With: map[string]interface{}{},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create runner options
			opts := RunnerOptions{
				WorkspaceRoot: filepath.Join(tempDir, "workspace"),
				CacheDir:      filepath.Join(tempDir, "cache"),
			}

			// Create runner
			runner, err := NewRunner(opts)
			if err != nil {
				t.Fatalf("Failed to create runner: %v", err)
			}
			defer runner.Close()

			// Replace the orchestrator with our mock
			mockDiscoverer := &MockDiscoverer{
				subscriptions: tt.mockSubscriptions,
				err:           tt.mockErr,
			}
			mockOrchestrator, _ := NewOrchestrator(mockDiscoverer)
			runner.orchestrator = mockOrchestrator

			// Execute the built-in step
			ctx := context.Background()
			result, err := runner.executeBuiltinStep(ctx, tt.step, tt.step.ID, runner.state.StartTime)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if result.Success {
					t.Errorf("Expected failure but got success")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			// Check success expectation
			if tt.expectSuccess {
				if !result.Success {
					t.Errorf("Expected success but got failure: %v", result.Error)
				}
			}

			// For fan-out steps, verify orchestrator was called
			if tt.step.Uses == "tako/fan-out@v1" && !tt.expectError {
				if !mockDiscoverer.called {
					t.Errorf("Expected orchestrator to be called but it wasn't")
				}
				if eventType, ok := tt.step.With["event_type"].(string); ok {
					if mockDiscoverer.eventType != eventType {
						t.Errorf("Expected event type %s but got %s", eventType, mockDiscoverer.eventType)
					}
					expectedArtifact := "current-repo:default"
					if mockDiscoverer.artifact != expectedArtifact {
						t.Errorf("Expected artifact %s but got %s", expectedArtifact, mockDiscoverer.artifact)
					}
				}
			}
		})
	}
}
