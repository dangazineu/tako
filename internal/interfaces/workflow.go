package interfaces

import (
	"context"
)

// WorkflowRunner defines the interface for executing workflows.
// Implementations of this interface are responsible for running workflows
// in specific repositories with provided inputs.
type WorkflowRunner interface {
	// ExecuteWorkflow executes a workflow in the specified repository.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - repoPath: Path to the repository where the workflow should be executed
	//   - workflowName: Name of the workflow to execute
	//   - inputs: Map of input parameters for the workflow
	//
	// Returns:
	//   - *ExecutionResult: Result of the workflow execution
	//   - error: An error if the execution fails
	ExecuteWorkflow(ctx context.Context, repoPath, workflowName string, inputs map[string]string) (*ExecutionResult, error)
}
