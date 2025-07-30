// Package steps contains the implementation of various workflow step types.
//
// This package provides step executors that implement specific workflow actions
// using the interfaces defined in the interfaces package. This design enables:
//
//   - Dependency injection for improved testability
//   - Clean separation between step logic and engine implementations
//   - Consistent patterns for future step implementations
//
// The main components in this package are:
//
//   - FanOutStepExecutor: Handles tako/fan-out@v1 step execution
//   - FanOutStepParams: Parameter structure for fan-out steps
//   - FanOutStepResult: Result structure for fan-out step execution
package steps

import (
	"time"

	"github.com/dangazineu/tako/internal/interfaces"
)

// FanOutStepParams represents the parameters for the tako/fan-out@v1 step.
type FanOutStepParams struct {
	EventType        string                 `yaml:"event_type"`
	WaitForChildren  bool                   `yaml:"wait_for_children"`
	Timeout          string                 `yaml:"timeout"`
	ConcurrencyLimit int                    `yaml:"concurrency_limit"`
	Payload          map[string]interface{} `yaml:"payload"`
	SchemaVersion    string                 `yaml:"schema_version"`
}

// FanOutStepResult represents the result of a fan-out execution.
type FanOutStepResult struct {
	Success          bool
	EventEmitted     bool
	SubscribersFound int
	TriggeredCount   int
	Errors           []string
	StartTime        time.Time
	EndTime          time.Time
	FanOutID         string // ID of the fan-out state for tracking
}

// FanOutStepExecutor handles the execution of tako/fan-out@v1 steps.
// It uses the provided interfaces to discover subscribers and trigger workflows.
type FanOutStepExecutor struct {
	discoverer interfaces.SubscriptionDiscoverer
	runner     interfaces.WorkflowRunner
}

// NewFanOutStepExecutor creates a new fan-out step executor with the provided dependencies.
func NewFanOutStepExecutor(discoverer interfaces.SubscriptionDiscoverer, runner interfaces.WorkflowRunner) *FanOutStepExecutor {
	return &FanOutStepExecutor{
		discoverer: discoverer,
		runner:     runner,
	}
}
