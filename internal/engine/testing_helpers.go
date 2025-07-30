package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dangazineu/tako/internal/interfaces"
)

// mockWorkflowRunner is a shared test implementation of interfaces.WorkflowRunner
type testMockWorkflowRunner struct{}

// NewTestMockWorkflowRunner creates a new mock workflow runner for testing
func NewTestMockWorkflowRunner() interfaces.WorkflowRunner {
	return &testMockWorkflowRunner{}
}

func (m *testMockWorkflowRunner) ExecuteWorkflow(ctx context.Context, repoPath, workflowName string, inputs map[string]string) (*interfaces.ExecutionResult, error) {
	// For backward compatibility with existing tests, fail if repository name contains "fail"
	if strings.Contains(repoPath, "fail") {
		return nil, fmt.Errorf("simulated failure for repository %s", repoPath)
	}

	// Simulate successful execution
	return &interfaces.ExecutionResult{
		RunID:     fmt.Sprintf("mock-run-%d", time.Now().Unix()),
		Success:   true,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(10 * time.Millisecond),
		Steps: []interfaces.StepResult{
			{
				ID:        "mock-step",
				Success:   true,
				Output:    "mock output",
				StartTime: time.Now(),
				EndTime:   time.Now().Add(10 * time.Millisecond),
			},
		},
	}, nil
}
