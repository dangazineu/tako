package steps

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/interfaces"
)

// FanOutStepParams represents the parameters for the tako/fan-out@v1 step.
type FanOutStepParams struct {
	EventType        string `json:"event_type"`
	WaitForChildren  bool   `json:"wait_for_children"`
	Timeout          string `json:"timeout"`
	ConcurrencyLimit int    `json:"concurrency_limit"`
}

// FanOutStepResult represents the result of executing a fan-out step.
type FanOutStepResult struct {
	EventEmitted       bool                `json:"event_emitted"`
	TriggeredWorkflows []TriggeredWorkflow `json:"triggered_workflows"`
	ChildRunIDs        []string            `json:"child_run_ids"`
	CompletedChildren  int                 `json:"completed_children"`
	FailedChildren     int                 `json:"failed_children"`
	TimedOut           bool                `json:"timed_out"`
}

// TriggeredWorkflow represents a workflow that was triggered by the fan-out step.
type TriggeredWorkflow struct {
	RepositoryName string `json:"repository_name"`
	RepositoryPath string `json:"repository_path"`
	WorkflowName   string `json:"workflow_name"`
	RunID          string `json:"run_id"`
	Status         string `json:"status"`
}

// FanOutExecutor executes tako/fan-out@v1 steps.
type FanOutExecutor struct {
	orchestrator interfaces.SubscriptionDiscoverer
	runner       interfaces.WorkflowRunner
}

// NewFanOutExecutor creates a new fan-out step executor.
func NewFanOutExecutor(orchestrator interfaces.SubscriptionDiscoverer, runner interfaces.WorkflowRunner) *FanOutExecutor {
	return &FanOutExecutor{
		orchestrator: orchestrator,
		runner:       runner,
	}
}

// Execute executes a tako/fan-out@v1 step.
func (f *FanOutExecutor) Execute(ctx context.Context, step config.WorkflowStep, artifactRef string, eventPayload map[string]string) (*FanOutStepResult, error) {
	// Parse step parameters
	params, err := f.parseParameters(step)
	if err != nil {
		return nil, fmt.Errorf("failed to parse fan-out parameters: %w", err)
	}

	result := &FanOutStepResult{
		EventEmitted:       false,
		TriggeredWorkflows: []TriggeredWorkflow{},
		ChildRunIDs:        []string{},
	}

	// Discover repositories with matching subscriptions
	matches, err := f.orchestrator.DiscoverSubscriptions(ctx, params.EventType, artifactRef, eventPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to discover subscriptions: %w", err)
	}

	// Mark event as emitted
	result.EventEmitted = true

	// If no matches found, return early
	if len(matches) == 0 {
		return result, nil
	}

	// Trigger workflows in matching repositories
	for _, match := range matches {
		// Map event payload to workflow inputs
		workflowInputs := f.mapEventToInputs(match.Subscription.Inputs, eventPayload)

		// Execute workflow (fire-and-forget mode for now)
		runID, err := f.runner.ExecuteChildWorkflow(ctx, match.RepositoryPath, match.Subscription.Workflow, workflowInputs)
		if err != nil {
			// Log error but continue with other repositories
			result.TriggeredWorkflows = append(result.TriggeredWorkflows, TriggeredWorkflow{
				RepositoryName: match.RepositoryName,
				RepositoryPath: match.RepositoryPath,
				WorkflowName:   match.Subscription.Workflow,
				RunID:          "",
				Status:         "failed",
			})
			continue
		}

		result.TriggeredWorkflows = append(result.TriggeredWorkflows, TriggeredWorkflow{
			RepositoryName: match.RepositoryName,
			RepositoryPath: match.RepositoryPath,
			WorkflowName:   match.Subscription.Workflow,
			RunID:          runID,
			Status:         "triggered",
		})
		result.ChildRunIDs = append(result.ChildRunIDs, runID)
	}

	// TODO: Implement deep synchronization when wait_for_children is true
	// For now, we only support fire-and-forget mode

	return result, nil
}

// parseParameters extracts and validates fan-out step parameters.
func (f *FanOutExecutor) parseParameters(step config.WorkflowStep) (*FanOutStepParams, error) {
	params := &FanOutStepParams{
		WaitForChildren:  false, // Default to fire-and-forget
		ConcurrencyLimit: 0,     // Default to unlimited
	}

	// Extract event_type (required)
	eventType, exists := step.With["event_type"]
	if !exists {
		return nil, fmt.Errorf("event_type parameter is required")
	}
	eventTypeStr, ok := eventType.(string)
	if !ok {
		return nil, fmt.Errorf("event_type must be a string")
	}
	params.EventType = eventTypeStr

	// Extract wait_for_children (optional)
	if waitValue, exists := step.With["wait_for_children"]; exists {
		if waitBool, ok := waitValue.(bool); ok {
			params.WaitForChildren = waitBool
		} else {
			return nil, fmt.Errorf("wait_for_children must be a boolean")
		}
	}

	// Extract timeout (optional)
	if timeoutValue, exists := step.With["timeout"]; exists {
		if timeoutStr, ok := timeoutValue.(string); ok {
			params.Timeout = timeoutStr
		} else {
			return nil, fmt.Errorf("timeout must be a string")
		}
	}

	// Extract concurrency_limit (optional)
	if concurrencyValue, exists := step.With["concurrency_limit"]; exists {
		switch v := concurrencyValue.(type) {
		case int:
			params.ConcurrencyLimit = v
		case float64:
			params.ConcurrencyLimit = int(v)
		case string:
			if limit, err := strconv.Atoi(v); err == nil {
				params.ConcurrencyLimit = limit
			} else {
				return nil, fmt.Errorf("concurrency_limit must be a number")
			}
		default:
			return nil, fmt.Errorf("concurrency_limit must be a number")
		}
	}

	// Validate parameters
	if err := f.validateParameters(params); err != nil {
		return nil, err
	}

	return params, nil
}

// validateParameters validates the parsed parameters.
func (f *FanOutExecutor) validateParameters(params *FanOutStepParams) error {
	if params.EventType == "" {
		return fmt.Errorf("event_type cannot be empty")
	}

	// Validate timeout format if provided
	if params.Timeout != "" {
		if _, err := time.ParseDuration(params.Timeout); err != nil {
			return fmt.Errorf("invalid timeout format '%s': %w", params.Timeout, err)
		}
	}

	// Validate concurrency limit
	if params.ConcurrencyLimit < 0 {
		return fmt.Errorf("concurrency_limit cannot be negative")
	}

	return nil
}

// mapEventToInputs maps event payload to workflow inputs based on subscription input mappings.
func (f *FanOutExecutor) mapEventToInputs(inputMappings map[string]string, eventPayload map[string]string) map[string]string {
	workflowInputs := make(map[string]string)

	for inputName, inputMapping := range inputMappings {
		// For now, implement simple direct mapping from event payload
		// TODO: Implement template expansion for complex mappings
		if value, exists := eventPayload[inputMapping]; exists {
			workflowInputs[inputName] = value
		} else {
			// Use the mapping as literal value if not found in payload
			workflowInputs[inputName] = inputMapping
		}
	}

	return workflowInputs
}
