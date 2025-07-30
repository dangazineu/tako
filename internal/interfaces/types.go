package interfaces

import (
	"time"

	"github.com/dangazineu/tako/internal/config"
)

// SubscriptionMatch represents a repository that subscribes to a specific event.
// This type is used by SubscriptionDiscoverer implementations.
type SubscriptionMatch struct {
	Repository   string              // Repository name (owner/repo format)
	Subscription config.Subscription // The matching subscription
	RepoPath     string              // Local path to the repository
}

// ExecutionResult represents the result of a workflow execution.
// This type is used by WorkflowRunner implementations.
type ExecutionResult struct {
	RunID     string
	Success   bool
	Error     error
	StartTime time.Time
	EndTime   time.Time
	Steps     []StepResult
}

// StepResult represents the result of a single step execution.
type StepResult struct {
	ID        string
	Success   bool
	Error     error
	StartTime time.Time
	EndTime   time.Time
	Output    string
	Outputs   map[string]string
}
