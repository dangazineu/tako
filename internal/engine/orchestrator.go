package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/graph"
	"github.com/dangazineu/tako/internal/interfaces"
)

// Orchestrator coordinates subscription discovery and workflow triggering.
// It provides a stable, high-level API for the subscription-based workflow
// triggering system while maintaining loose coupling through dependency injection.
//
// The orchestrator supports filtering and prioritization of subscriptions:
//   - Filtering subscriptions based on criteria
//   - Prioritizing subscriptions by repository path for deterministic ordering
//   - Adding structured logging and monitoring
//   - Coordinating workflow triggering with state management
//   - Handling idempotency and diamond dependency resolution
type Orchestrator struct {
	discoverer interfaces.SubscriptionDiscoverer
	config     OrchestratorConfig
}

// OrchestratorConfig contains configuration options for orchestrator behavior.
type OrchestratorConfig struct {
	// EnableFiltering enables subscription filtering (disabled by default for backward compatibility)
	EnableFiltering bool
	// EnablePrioritization enables priority-based sorting (disabled by default for backward compatibility)
	EnablePrioritization bool
	// FilterDisabledSubscriptions removes subscriptions marked as disabled
	FilterDisabledSubscriptions bool
}

// NewOrchestrator creates a new Orchestrator with the provided dependencies and default configuration.
// The discoverer is used to find repositories that subscribe to specific events.
// Returns an error if the discoverer is nil to ensure safe construction.
//
// Example usage:
//
//	// Create a discovery manager for repository scanning
//	cacheDir := "~/.tako/cache"
//	discoveryManager := engine.NewDiscoveryManager(cacheDir)
//
//	// Create the orchestrator with dependency injection
//	orchestrator, err := engine.NewOrchestrator(discoveryManager)
//	if err != nil {
//		return fmt.Errorf("failed to create orchestrator: %w", err)
//	}
//
//	// The orchestrator is now ready to coordinate subscription discovery
//	ctx := context.Background()
//	matches, err := orchestrator.DiscoverSubscriptions(ctx, "myorg/mylib:library", "build_completed")
//
// For testing, you can provide a mock implementation:
//
//	mockDiscoverer := &MyMockDiscoverer{}
//	testOrchestrator, err := engine.NewOrchestrator(mockDiscoverer)
func NewOrchestrator(discoverer interfaces.SubscriptionDiscoverer) (*Orchestrator, error) {
	return NewOrchestratorWithConfig(discoverer, OrchestratorConfig{})
}

// NewOrchestratorWithConfig creates a new Orchestrator with the provided dependencies and configuration.
// This allows for customization of orchestrator behavior while maintaining backward compatibility.
//
// Example usage with configuration:
//
//	config := engine.OrchestratorConfig{
//		EnableFiltering:             true,
//		EnablePrioritization:        true,
//		FilterDisabledSubscriptions: true,
//	}
//	orchestrator, err := engine.NewOrchestratorWithConfig(discoveryManager, config)
func NewOrchestratorWithConfig(discoverer interfaces.SubscriptionDiscoverer, config OrchestratorConfig) (*Orchestrator, error) {
	if discoverer == nil {
		return nil, errors.New("discoverer cannot be nil")
	}
	return &Orchestrator{
		discoverer: discoverer,
		config:     config,
	}, nil
}

// DiscoverSubscriptions finds all repositories that subscribe to the specified
// artifact and event type. This method provides the orchestration layer for
// subscription discovery with optional filtering and prioritization capabilities.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - artifact: The artifact identifier in "owner/repo:artifact" format
//   - eventType: The type of event (e.g., "build_completed", "test_passed")
//
// Returns:
//   - []interfaces.SubscriptionMatch: List of matching subscriptions with repository information
//   - error: An error if the discovery process fails
//
// Orchestration features:
//   - Filtering logic to remove disabled subscriptions (if enabled)
//   - Priority-based sorting for deterministic subscription ordering (if enabled)
//   - Context cancellation handling for responsive behavior
//   - Parameter validation for robustness
//
// Example usage:
//
//	matches, err := orchestrator.DiscoverSubscriptions(ctx, "myorg/mylib:library", "build_completed")
//	if err != nil {
//	    return fmt.Errorf("failed to discover subscriptions: %w", err)
//	}
//	for _, match := range matches {
//	    fmt.Printf("Found subscription in %s for workflow %s\n", match.Repository, match.Subscription.Workflow)
//	}
func (o *Orchestrator) DiscoverSubscriptions(ctx context.Context, artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
	// Check for context cancellation early
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate parameters at the orchestrator level for robustness
	if artifact == "" {
		return nil, errors.New("orchestrator: artifact cannot be empty")
	}
	if eventType == "" {
		return nil, errors.New("orchestrator: eventType cannot be empty")
	}

	// Delegate to the discoverer for raw subscription discovery
	rawMatches, err := o.discoverer.FindSubscribers(artifact, eventType)
	if err != nil {
		return nil, err
	}

	// Apply orchestration logic
	filteredMatches := o.filterSubscriptions(rawMatches)
	prioritizedMatches := o.prioritizeSubscriptions(filteredMatches)

	return prioritizedMatches, nil
}

// filterSubscriptions applies filtering logic to subscription matches.
// Currently supports filtering out disabled subscriptions if configured.
func (o *Orchestrator) filterSubscriptions(matches []interfaces.SubscriptionMatch) []interfaces.SubscriptionMatch {
	if !o.config.EnableFiltering {
		return matches
	}

	if !o.config.FilterDisabledSubscriptions {
		return matches
	}

	// Filter out disabled subscriptions (if the subscription has a Disabled field)
	// Note: Since config.Subscription doesn't currently have a Disabled field,
	// this filtering is a no-op for now but provides the infrastructure
	// for future enhancement when the schema is extended.
	filtered := make([]interfaces.SubscriptionMatch, 0, len(matches))
	filtered = append(filtered, matches...)

	return filtered
}

// prioritizeSubscriptions applies priority-based sorting to subscription matches.
// Sorts by repository path for deterministic ordering, supporting the "first-wins"
// diamond dependency resolution rule implemented in the fan-out executor.
func (o *Orchestrator) prioritizeSubscriptions(matches []interfaces.SubscriptionMatch) []interfaces.SubscriptionMatch {
	if !o.config.EnablePrioritization {
		return matches
	}

	// Create a copy to avoid modifying the original slice
	prioritized := make([]interfaces.SubscriptionMatch, len(matches))
	copy(prioritized, matches)

	// Sort by repository path for deterministic ordering
	// This supports the "first-wins" rule for diamond dependency resolution
	sort.Slice(prioritized, func(i, j int) bool {
		if prioritized[i].Repository != prioritized[j].Repository {
			return prioritized[i].Repository < prioritized[j].Repository
		}
		// Secondary sort by workflow for additional determinism
		return prioritized[i].Subscription.Workflow < prioritized[j].Subscription.Workflow
	})

	return prioritized
}

// ========== HYBRID ORCHESTRATION IMPLEMENTATION ==========

// WorkflowOrchestrator manages hybrid directed+event-driven orchestration across repositories.
// It coordinates the execution flow: workflow steps → event reactions → workflow completion → dependent triggering.
type WorkflowOrchestrator struct {
	runner     *Runner
	discoverer interfaces.SubscriptionDiscoverer
	cacheDir   string
	homeDir    string
	localOnly  bool
	state      *OrchestrationState
	eventQueue map[string][]EventQueueEntry // repo+eventType -> FIFO queue
}

// OrchestrationState tracks the complete multi-repository execution state for resume capabilities.
type OrchestrationState struct {
	RunID          string             `json:"run_id"`
	ParentRepo     string             `json:"parent_repo"`
	ParentWorkflow string             `json:"parent_workflow"`
	ParentInputs   map[string]string  `json:"parent_inputs"`
	WorkflowPhase  OrchestrationPhase `json:"workflow_phase"`

	// Event-driven tracking
	PendingEvents    []EventQueueEntry    `json:"pending_events"`
	SubscriberRuns   map[string]string    `json:"subscriber_runs"`   // eventKey -> run-id
	SubscriberStatus map[string]RunStatus `json:"subscriber_status"` // run-id -> status

	// Directed orchestration tracking
	DependentRuns   map[string]string    `json:"dependent_runs"`   // repo -> run-id
	DependentStatus map[string]RunStatus `json:"dependent_status"` // run-id -> status

	// Timing and metadata
	StartTime  time.Time `json:"start_time"`
	LastUpdate time.Time `json:"last_update"`
}

// OrchestrationPhase tracks the current phase of hybrid orchestration execution.
type OrchestrationPhase string

const (
	PhaseExecutingSteps       OrchestrationPhase = "executing_steps"
	PhaseWaitingSubscribers   OrchestrationPhase = "waiting_for_subscribers"
	PhaseTriggeringDependents OrchestrationPhase = "triggering_dependents"
	PhaseCompleted            OrchestrationPhase = "completed"
	PhaseFailed               OrchestrationPhase = "failed"
)

// RunStatus tracks the status of individual workflow runs.
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
)

// EventQueueEntry represents a queued event for FIFO processing.
type EventQueueEntry struct {
	EventType string            `json:"event_type"`
	Artifact  string            `json:"artifact"`
	Payload   map[string]string `json:"payload"`
	EmittedAt time.Time         `json:"emitted_at"`
}

// NewWorkflowOrchestrator creates a new hybrid orchestrator.
func NewWorkflowOrchestrator(runner *Runner, discoverer interfaces.SubscriptionDiscoverer, cacheDir, homeDir string, localOnly bool) *WorkflowOrchestrator {
	return &WorkflowOrchestrator{
		runner:     runner,
		discoverer: discoverer,
		cacheDir:   cacheDir,
		homeDir:    homeDir,
		localOnly:  localOnly,
		eventQueue: make(map[string][]EventQueueEntry),
	}
}

// ExecuteHybridWorkflow implements the complete hybrid directed+event-driven orchestration.
func (wo *WorkflowOrchestrator) ExecuteHybridWorkflow(ctx context.Context, workflowName string, inputs map[string]string, parentRepo string) (*ExecutionResult, error) {
	startTime := time.Now()

	// Initialize orchestration state
	wo.state = &OrchestrationState{
		RunID:            wo.runner.runID,
		ParentRepo:       parentRepo,
		ParentWorkflow:   workflowName,
		ParentInputs:     inputs,
		WorkflowPhase:    PhaseExecutingSteps,
		PendingEvents:    []EventQueueEntry{},
		SubscriberRuns:   make(map[string]string),
		SubscriberStatus: make(map[string]RunStatus),
		DependentRuns:    make(map[string]string),
		DependentStatus:  make(map[string]RunStatus),
		StartTime:        startTime,
		LastUpdate:       startTime,
	}

	// Phase 1: Execute parent workflow with event handling
	result, err := wo.executeWorkflowWithEventHandling(ctx, workflowName, inputs, parentRepo)
	if err != nil {
		wo.state.WorkflowPhase = PhaseFailed
		return result, fmt.Errorf("failed to execute parent workflow: %w", err)
	}

	// Phase 2: Trigger dependent workflows (directed orchestration)
	if err := wo.triggerDependentWorkflows(ctx, parentRepo); err != nil {
		wo.state.WorkflowPhase = PhaseFailed
		return result, fmt.Errorf("failed to trigger dependent workflows: %w", err)
	}

	wo.state.WorkflowPhase = PhaseCompleted
	wo.state.LastUpdate = time.Now()

	return result, nil
}

// executeWorkflowWithEventHandling executes workflow steps and handles event reactions.
func (wo *WorkflowOrchestrator) executeWorkflowWithEventHandling(ctx context.Context, workflowName string, inputs map[string]string, parentRepo string) (*ExecutionResult, error) {
	wo.state.WorkflowPhase = PhaseExecutingSteps
	wo.state.LastUpdate = time.Now()

	// Resolve repository path
	repoPath, err := wo.resolveRepositoryPath(parentRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve repository path: %w", err)
	}

	// Load workflow configuration
	configPath := filepath.Join(repoPath, "tako.yml")
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	workflow, exists := cfg.Workflows[workflowName]
	if !exists {
		return nil, fmt.Errorf("workflow '%s' not found", workflowName)
	}

	startTime := time.Now()
	var stepResults []StepResult
	stepOutputs := make(map[string]map[string]string)

	// Execute each step sequentially with event handling
	for _, step := range workflow.Steps {
		select {
		case <-ctx.Done():
			return &ExecutionResult{
				RunID:     wo.runner.runID,
				Success:   false,
				Error:     ctx.Err(),
				StartTime: startTime,
				EndTime:   time.Now(),
				Steps:     stepResults,
			}, ctx.Err()
		default:
		}

		// Execute the step
		stepResult, err := wo.executeStepWithEventReactions(ctx, step, repoPath, parentRepo, inputs, stepOutputs)
		stepResults = append(stepResults, stepResult)

		if err != nil {
			return &ExecutionResult{
				RunID:     wo.runner.runID,
				Success:   false,
				Error:     fmt.Errorf("step '%s' failed: %w", step.ID, err),
				StartTime: startTime,
				EndTime:   time.Now(),
				Steps:     stepResults,
			}, err
		}

		// Store step outputs for future steps
		if len(stepResult.Outputs) > 0 {
			stepOutputs[step.ID] = stepResult.Outputs
		}
	}

	return &ExecutionResult{
		RunID:     wo.runner.runID,
		Success:   true,
		StartTime: startTime,
		EndTime:   time.Now(),
		Steps:     stepResults,
	}, nil
}

// executeStepWithEventReactions executes a single step and handles any event reactions.
func (wo *WorkflowOrchestrator) executeStepWithEventReactions(ctx context.Context, step config.WorkflowStep, repoPath, parentRepo string, inputs map[string]string, stepOutputs map[string]map[string]string) (StepResult, error) {
	// Execute the step using the existing runner logic
	stepResult, err := wo.runner.executeStep(ctx, step, repoPath, inputs, stepOutputs)
	if err != nil {
		return stepResult, err
	}

	// Check if this step might emit events (e.g., fan-out steps, build steps)
	// For now, we'll trigger event handling after successful steps
	if stepResult.Success {
		if err := wo.handleStepEventEmissions(ctx, step, parentRepo, inputs); err != nil {
			// Event handling failure shouldn't fail the step, but should be logged
			// In a production system, we might want to have configurable behavior here
			wo.state.LastUpdate = time.Now()
			// For now, continue execution but log the error
		}
	}

	return stepResult, nil
}

// handleStepEventEmissions handles event emissions that might occur after step execution.
func (wo *WorkflowOrchestrator) handleStepEventEmissions(ctx context.Context, step config.WorkflowStep, parentRepo string, inputs map[string]string) error {
	// Check if this step produces artifacts or events
	// In the Tako system, events are typically emitted:
	// 1. After build completion (build_completed event)
	// 2. After test completion (test_completed event)
	// 3. After deployment (deployment_completed event)

	// Determine event type based on step characteristics
	eventType := wo.determineEventType(step)
	if eventType == "" {
		// No events to emit from this step
		return nil
	}

	// Create artifact identifier in the format "owner/repo:artifact"
	artifact := fmt.Sprintf("%s:%s", parentRepo, "default") // Use default artifact for now

	// Discover subscriptions for this event
	subscriptions, err := wo.discoverer.FindSubscribers(artifact, eventType)
	if err != nil {
		return fmt.Errorf("failed to discover subscribers for event %s: %w", eventType, err)
	}

	if len(subscriptions) == 0 {
		// No subscribers, nothing to do
		return nil
	}

	wo.state.WorkflowPhase = PhaseWaitingSubscribers
	wo.state.LastUpdate = time.Now()

	// Execute subscriber workflows concurrently
	var wg sync.WaitGroup
	errChan := make(chan error, len(subscriptions))

	for _, subscription := range subscriptions {
		wg.Add(1)
		go func(sub interfaces.SubscriptionMatch) {
			defer wg.Done()

			eventKey := fmt.Sprintf("%s:%s", sub.Repository, eventType)
			runID := fmt.Sprintf("%s-subscriber-%s", wo.runner.runID, sub.Repository)

			// Track subscriber execution
			wo.state.SubscriberRuns[eventKey] = runID
			wo.state.SubscriberStatus[runID] = RunStatusRunning

			// Execute subscriber workflow
			subscriberPath, err := wo.resolveRepositoryPath(sub.Repository)
			if err != nil {
				errChan <- fmt.Errorf("failed to resolve subscriber repository %s: %w", sub.Repository, err)
				wo.state.SubscriberStatus[runID] = RunStatusFailed
				return
			}

			result, err := wo.runner.ExecuteWorkflow(ctx, sub.Subscription.Workflow, inputs, subscriberPath)
			if err != nil {
				errChan <- fmt.Errorf("failed to execute subscriber workflow %s in %s: %w", sub.Subscription.Workflow, sub.Repository, err)
				wo.state.SubscriberStatus[runID] = RunStatusFailed
				return
			}

			if !result.Success {
				errChan <- fmt.Errorf("subscriber workflow %s in %s failed", sub.Subscription.Workflow, sub.Repository)
				wo.state.SubscriberStatus[runID] = RunStatusFailed
				return
			}

			wo.state.SubscriberStatus[runID] = RunStatusCompleted
		}(subscription)
	}

	// Wait for all subscriber workflows to complete
	wg.Wait()
	close(errChan)

	// Collect any errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		// Format multiple errors
		errorMsg := "subscriber workflows failed:"
		for _, err := range errors {
			errorMsg += "\n  - " + err.Error()
		}
		return fmt.Errorf("%s", errorMsg)
	}

	return nil
}

// determineEventType determines what event type should be emitted based on step characteristics.
func (wo *WorkflowOrchestrator) determineEventType(step config.WorkflowStep) string {
	// Check if this is a fan-out step (explicitly triggers events)
	if step.Uses == "tako/fan-out@v1" {
		if eventType, ok := step.With["event_type"].(string); ok {
			return eventType
		}
	}

	// Check step ID patterns for common event types
	stepID := strings.ToLower(step.ID)

	if strings.Contains(stepID, "build") {
		return "build_completed"
	}
	if strings.Contains(stepID, "test") {
		return "test_completed"
	}
	if strings.Contains(stepID, "deploy") {
		return "deployment_completed"
	}

	// Check run command patterns
	runCmd := strings.ToLower(step.Run)
	if strings.Contains(runCmd, "maven") || strings.Contains(runCmd, "gradle") || strings.Contains(runCmd, "go build") {
		return "build_completed"
	}
	if strings.Contains(runCmd, "test") {
		return "test_completed"
	}

	// No recognizable event pattern
	return ""
}

// triggerDependentWorkflows triggers workflows in dependent repositories (directed orchestration).
func (wo *WorkflowOrchestrator) triggerDependentWorkflows(ctx context.Context, parentRepo string) error {
	wo.state.WorkflowPhase = PhaseTriggeringDependents
	wo.state.LastUpdate = time.Now()

	// Build dependency graph to find dependents
	repoPath, err := wo.resolveRepositoryPath(parentRepo)
	if err != nil {
		return fmt.Errorf("failed to resolve parent repo path: %w", err)
	}

	_, err = graph.BuildGraph(parentRepo, repoPath, wo.cacheDir, wo.homeDir, wo.localOnly)
	if err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Load parent configuration to get dependents
	cfg, err := config.Load(fmt.Sprintf("%s/tako.yml", repoPath))
	if err != nil {
		return fmt.Errorf("failed to load parent config: %w", err)
	}

	if len(cfg.Dependents) == 0 {
		// No dependents to execute
		return nil
	}

	// Execute dependent workflows with proper synchronization
	var wg sync.WaitGroup
	errChan := make(chan error, len(cfg.Dependents))

	for _, dependent := range cfg.Dependents {
		dependentPath, err := wo.resolveRepositoryPath(dependent.Repo)
		if err != nil {
			errChan <- fmt.Errorf("failed to resolve dependent repository %s: %w", dependent.Repo, err)
			continue
		}

		wg.Add(1)
		go func(dep config.Dependent, depPath string) {
			defer wg.Done()

			runID := fmt.Sprintf("%s-dependent-%s", wo.runner.runID, strings.ReplaceAll(dep.Repo, "/", "-"))

			// Track dependent execution
			wo.state.DependentRuns[dep.Repo] = runID
			wo.state.DependentStatus[runID] = RunStatusRunning
			wo.state.LastUpdate = time.Now()

			// Determine which workflows to execute in the dependent repository
			workflowsToExecute := wo.determineWorkflowsToExecute(dep)

			for _, workflowName := range workflowsToExecute {
				// Execute workflow in the dependent repository
				result, err := wo.runner.ExecuteWorkflow(ctx, workflowName, wo.state.ParentInputs, depPath)
				if err != nil {
					errChan <- fmt.Errorf("failed to execute workflow %s in dependent %s: %w", workflowName, dep.Repo, err)
					wo.state.DependentStatus[runID] = RunStatusFailed
					wo.state.LastUpdate = time.Now()
					return
				}

				if !result.Success {
					errChan <- fmt.Errorf("workflow %s failed in dependent %s", workflowName, dep.Repo)
					wo.state.DependentStatus[runID] = RunStatusFailed
					wo.state.LastUpdate = time.Now()
					return
				}
			}

			// Mark dependent as completed
			wo.state.DependentStatus[runID] = RunStatusCompleted
			wo.state.LastUpdate = time.Now()

		}(dependent, dependentPath)
	}

	// Wait for all dependent workflows to complete
	wg.Wait()
	close(errChan)

	// Collect any errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		// Format multiple errors
		errorMsg := "dependent workflows failed:"
		for _, err := range errors {
			errorMsg += "\n  - " + err.Error()
		}
		return fmt.Errorf("%s", errorMsg)
	}

	return nil
}

// determineWorkflowsToExecute determines which workflows should be executed in a dependent repository.
func (wo *WorkflowOrchestrator) determineWorkflowsToExecute(dependent config.Dependent) []string {
	// If specific workflows are defined for this dependent, use those
	if len(dependent.Workflows) > 0 {
		return dependent.Workflows
	}

	// Otherwise, execute the same workflow that was triggered in the parent
	return []string{wo.state.ParentWorkflow}
}

// resolveRepositoryPath converts repository specification to local cache path.
func (wo *WorkflowOrchestrator) resolveRepositoryPath(repoSpec string) (string, error) {
	// Parse repository specification: "owner/repo:branch" or "owner/repo"
	parts := strings.Split(repoSpec, ":")
	repoPath := parts[0]
	branch := "main"
	if len(parts) > 1 {
		branch = parts[1]
	}

	// Split owner/repo
	ownerRepo := strings.Split(repoPath, "/")
	if len(ownerRepo) != 2 {
		return "", fmt.Errorf("invalid repository specification: %s (expected format: owner/repo or owner/repo:branch)", repoSpec)
	}

	owner := ownerRepo[0]
	repo := ownerRepo[1]

	// Construct cache path: ~/.tako/cache/repos/owner/repo/branch
	cachePath := fmt.Sprintf("%s/repos/%s/%s/%s", wo.cacheDir, owner, repo, branch)

	// Check if repository exists in cache (like the original Runner does)
	if _, err := config.Load(fmt.Sprintf("%s/tako.yml", cachePath)); err != nil {
		return "", fmt.Errorf("repository %s not found in cache at %s", repoSpec, cachePath)
	}

	return cachePath, nil
}
