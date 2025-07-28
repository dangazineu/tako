package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dangazineu/tako/internal/config"
)

// FanOutExecutor handles the execution of tako/fan-out@v1 steps.
type FanOutExecutor struct {
	discoveryManager      *DiscoveryManager
	subscriptionEvaluator *SubscriptionEvaluator
	stateManager          *FanOutStateManager
	eventValidator        *EventValidator
	circuitBreakerManager *CircuitBreakerManager
	metricsCollector      *MetricsCollector
	healthChecker         *HealthChecker
	logger                Logger
	cacheDir              string
	debug                 bool

	// Configuration
	retryConfig          RetryConfig
	circuitBreakerConfig CircuitBreakerConfig
}

// NewFanOutExecutor creates a new fan-out executor.
func NewFanOutExecutor(cacheDir string, debug bool) (*FanOutExecutor, error) {
	discoveryManager := NewDiscoveryManager(cacheDir)

	subscriptionEvaluator, err := NewSubscriptionEvaluator()
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription evaluator: %v", err)
	}

	// Create state manager for tracking fan-out operations
	stateDir := filepath.Join(cacheDir, "fanout-states")
	stateManager, err := NewFanOutStateManager(stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create state manager: %v", err)
	}

	// Create event validator with common schemas
	eventValidator := NewEventValidator()
	if err := RegisterCommonSchemas(eventValidator); err != nil {
		return nil, fmt.Errorf("failed to register common schemas: %v", err)
	}

	// Initialize resilience and monitoring components
	circuitBreakerConfig := DefaultCircuitBreakerConfig()
	retryConfig := DefaultRetryConfig()

	circuitBreakerManager := NewCircuitBreakerManager(circuitBreakerConfig)
	metricsCollector := NewMetricsCollector()
	healthChecker := NewHealthChecker(metricsCollector, circuitBreakerManager)
	logger := NewStructuredLogger(debug)

	return &FanOutExecutor{
		discoveryManager:      discoveryManager,
		subscriptionEvaluator: subscriptionEvaluator,
		stateManager:          stateManager,
		eventValidator:        eventValidator,
		circuitBreakerManager: circuitBreakerManager,
		metricsCollector:      metricsCollector,
		healthChecker:         healthChecker,
		logger:                logger,
		cacheDir:              cacheDir,
		debug:                 debug,
		retryConfig:           retryConfig,
		circuitBreakerConfig:  circuitBreakerConfig,
	}, nil
}

// FanOutParams represents the parameters for the tako/fan-out@v1 step.
type FanOutParams struct {
	EventType        string                 `yaml:"event_type"`
	WaitForChildren  bool                   `yaml:"wait_for_children"`
	Timeout          string                 `yaml:"timeout"`
	ConcurrencyLimit int                    `yaml:"concurrency_limit"`
	Payload          map[string]interface{} `yaml:"payload"`
	SchemaVersion    string                 `yaml:"schema_version"`
}

// FanOutResult represents the result of a fan-out execution.
type FanOutResult struct {
	Success          bool
	EventEmitted     bool
	SubscribersFound int
	TriggeredCount   int
	Errors           []string
	StartTime        time.Time
	EndTime          time.Time
	FanOutID         string // ID of the fan-out state for tracking
}

// Execute performs the fan-out operation with proper state management.
func (fe *FanOutExecutor) Execute(step config.WorkflowStep, sourceRepo string) (*FanOutResult, error) {
	return fe.ExecuteWithContext(step, sourceRepo, "")
}

// ExecuteWithContext performs the fan-out operation with optional parent run context.
func (fe *FanOutExecutor) ExecuteWithContext(step config.WorkflowStep, sourceRepo, parentRunID string) (*FanOutResult, error) {
	startTime := time.Now()
	result := &FanOutResult{
		StartTime: startTime,
		Errors:    []string{},
	}

	// Record metrics
	fe.metricsCollector.RecordFanOutStarted()
	defer func() {
		duration := time.Since(startTime)
		success := len(result.Errors) == 0
		fe.metricsCollector.RecordFanOutCompleted(duration, success, result.TriggeredCount)

		// Structured logging
		fe.logger.Info("Fan-out completed",
			"duration_ms", duration.Milliseconds(),
			"success", success,
			"triggered_count", result.TriggeredCount,
			"error_count", len(result.Errors),
			"source_repo", sourceRepo,
			"fan_out_id", result.FanOutID,
		)
	}()

	// Parse fan-out parameters
	params, err := fe.parseFanOutParams(step.With)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("invalid parameters: %v", err))
		result.EndTime = time.Now()
		return result, err
	}

	// Create fan-out state for tracking
	fanOutID := fmt.Sprintf("fanout-%d-%s", startTime.Unix(), params.EventType)
	result.FanOutID = fanOutID

	var timeout time.Duration
	if params.Timeout != "" {
		timeout, err = time.ParseDuration(params.Timeout)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("invalid timeout format: %v", err))
			result.EndTime = time.Now()
			return result, err
		}
	}

	state, err := fe.stateManager.CreateFanOutState(fanOutID, parentRunID, sourceRepo, params.EventType, params.WaitForChildren, timeout)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to create fan-out state: %v", err))
		result.EndTime = time.Now()
		return result, err
	}

	// Start the fan-out operation
	state.StartFanOut()

	if fe.debug {
		fmt.Printf("Fan-out step: emitting event '%s' from '%s' (ID: %s)\n", params.EventType, sourceRepo, fanOutID)
	}

	// Create enhanced event from parameters
	enhancedEvent := NewEventBuilder(params.EventType).
		WithSource(sourceRepo).
		WithPayload(params.Payload).
		Build()

	// Set schema if provided
	if params.SchemaVersion != "" {
		enhancedEvent.Schema = fmt.Sprintf("%s@%s", params.EventType, params.SchemaVersion)
	}

	// Apply defaults and validate event if schema is specified
	if enhancedEvent.Schema != "" {
		if err := fe.eventValidator.ApplyDefaults(&enhancedEvent); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to apply event defaults: %v", err))
			result.EndTime = time.Now()
			return result, err
		}

		if err := fe.eventValidator.ValidateEvent(enhancedEvent); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("event validation failed: %v", err))
			result.EndTime = time.Now()
			return result, err
		}

		if fe.debug {
			fmt.Printf("Event validated against schema '%s'\n", enhancedEvent.Schema)
		}
	}

	// Convert to legacy event for backward compatibility with existing code
	event := enhancedEvent.ToLegacyEvent()

	result.EventEmitted = true

	// Find subscribers for this event
	artifact := fmt.Sprintf("%s:default", sourceRepo)
	subscribers, err := fe.discoveryManager.FindSubscribers(artifact, params.EventType)
	if err != nil {
		state.FailFanOut(fmt.Sprintf("failed to find subscribers: %v", err))
		result.Errors = append(result.Errors, fmt.Sprintf("failed to find subscribers: %v", err))
		result.EndTime = time.Now()
		return result, err
	}

	result.SubscribersFound = len(subscribers)

	if fe.debug {
		fmt.Printf("Found %d subscribers for event '%s'\n", len(subscribers), params.EventType)
	}

	// Filter subscribers using subscription evaluation
	validSubscribers := []SubscriptionMatch{}
	for _, subscriber := range subscribers {
		matches, err := fe.subscriptionEvaluator.EvaluateSubscription(subscriber.Subscription, event)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("subscription evaluation failed for %s: %v", subscriber.Repository, err))
			continue
		}
		if matches {
			validSubscribers = append(validSubscribers, subscriber)
		}
	}

	if fe.debug {
		fmt.Printf("After filtering: %d valid subscribers\n", len(validSubscribers))
	}

	// Trigger subscribers with state tracking
	if len(validSubscribers) > 0 {
		triggeredCount, errors := fe.triggerSubscribersWithState(validSubscribers, event, params, state)
		result.TriggeredCount = triggeredCount
		result.Errors = append(result.Errors, errors...)
	}

	// Handle waiting for children
	if params.WaitForChildren {
		if result.TriggeredCount > 0 {
			if fe.debug {
				fmt.Printf("Waiting for %d child workflows to complete\n", result.TriggeredCount)
			}

			// Start waiting state
			state.StartWaiting()

			// Check if already complete (for simulation case)
			if state.IsComplete() {
				if fe.debug {
					fmt.Printf("All children already completed\n")
				}
			} else {
				if fe.debug {
					summary := state.GetSummary()
					fmt.Printf("State not complete yet: status=%s, total=%d, completed=%d, running=%d, pending=%d\n",
						summary.Status, summary.TotalChildren, summary.CompletedChildren, summary.RunningChildren, summary.PendingChildren)
				}
				// Wait for completion with timeout
				err := fe.waitForChildrenWithState(state, timeout)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("wait for children failed: %v", err))
				}
			}
		} else {
			// No children to wait for, complete immediately
			state.CompleteFanOut()
		}
	} else {
		// Not waiting for children, complete immediately
		state.CompleteFanOut()
	}

	result.Success = len(result.Errors) == 0
	result.EndTime = time.Now()

	if fe.debug {
		fmt.Printf("Fan-out completed: success=%v, triggered=%d, errors=%d\n",
			result.Success, result.TriggeredCount, len(result.Errors))
	}

	return result, nil
}

// parseFanOutParams parses the fan-out step parameters from the step's with map.
func (fe *FanOutExecutor) parseFanOutParams(withParams map[string]interface{}) (*FanOutParams, error) {
	params := &FanOutParams{
		WaitForChildren:  false,
		ConcurrencyLimit: 0, // 0 means no limit
		Payload:          make(map[string]interface{}),
	}

	// Required: event_type
	if eventType, ok := withParams["event_type"]; ok {
		if eventTypeStr, ok := eventType.(string); ok {
			params.EventType = eventTypeStr
		} else {
			return nil, fmt.Errorf("event_type must be a string")
		}
	} else {
		return nil, fmt.Errorf("event_type is required")
	}

	// Optional: wait_for_children
	if waitForChildren, ok := withParams["wait_for_children"]; ok {
		if waitBool, ok := waitForChildren.(bool); ok {
			params.WaitForChildren = waitBool
		} else {
			return nil, fmt.Errorf("wait_for_children must be a boolean")
		}
	}

	// Optional: timeout
	if timeout, ok := withParams["timeout"]; ok {
		if timeoutStr, ok := timeout.(string); ok {
			params.Timeout = timeoutStr
		} else {
			return nil, fmt.Errorf("timeout must be a string")
		}
	}

	// Optional: concurrency_limit
	if concurrencyLimit, ok := withParams["concurrency_limit"]; ok {
		if concurrencyInt, ok := concurrencyLimit.(int); ok {
			params.ConcurrencyLimit = concurrencyInt
		} else if concurrencyStr, ok := concurrencyLimit.(string); ok {
			// Handle string numbers
			if parsed, err := strconv.Atoi(concurrencyStr); err == nil {
				params.ConcurrencyLimit = parsed
			} else {
				return nil, fmt.Errorf("concurrency_limit must be an integer")
			}
		} else {
			return nil, fmt.Errorf("concurrency_limit must be an integer")
		}
	}

	// Optional: payload
	if payload, ok := withParams["payload"]; ok {
		if payloadMap, ok := payload.(map[string]interface{}); ok {
			params.Payload = payloadMap
		} else {
			return nil, fmt.Errorf("payload must be a map")
		}
	}

	// Optional: schema_version
	if schemaVersion, ok := withParams["schema_version"]; ok {
		if schemaVersionStr, ok := schemaVersion.(string); ok {
			params.SchemaVersion = schemaVersionStr
		} else {
			return nil, fmt.Errorf("schema_version must be a string")
		}
	}

	return params, nil
}

// triggerSubscribersWithState triggers workflows in subscriber repositories with state tracking.
func (fe *FanOutExecutor) triggerSubscribersWithState(subscribers []SubscriptionMatch, event Event, params *FanOutParams, state *FanOutState) (int, []string) {
	errors := []string{}
	triggeredCount := 0

	// Sort subscribers alphabetically for deterministic execution order
	sort.Slice(subscribers, func(i, j int) bool {
		return subscribers[i].Repository < subscribers[j].Repository
	})

	// Determine concurrency limit
	concurrencyLimit := params.ConcurrencyLimit
	if concurrencyLimit <= 0 {
		concurrencyLimit = len(subscribers) // No limit, run all in parallel
	}

	// Use semaphore pattern for concurrency control
	semaphore := make(chan struct{}, concurrencyLimit)
	var wg sync.WaitGroup
	var mutex sync.Mutex

	for _, subscriber := range subscribers {
		// Check for idempotency - skip if workflow already triggered
		isTriggered, existingRunID := state.IsWorkflowTriggered(subscriber.Repository, subscriber.Subscription.Workflow)
		if isTriggered {
			fe.logger.Info("Skipping already triggered workflow (idempotency)",
				"repository", subscriber.Repository,
				"workflow", subscriber.Subscription.Workflow,
				"existing_run_id", existingRunID,
			)
			continue
		}

		// Add child workflow to state before triggering
		workflowInputs, err := fe.subscriptionEvaluator.ProcessEventPayload(event.Payload, subscriber.Subscription)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to process payload for %s: %v", subscriber.Repository, err))
			continue
		}

		child := state.AddChildWorkflow(subscriber.Repository, subscriber.Subscription.Workflow, workflowInputs)

		wg.Add(1)
		go func(sub SubscriptionMatch, childWorkflow *ChildWorkflow) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Record child execution start
			childStartTime := time.Now()
			fe.metricsCollector.RecordChildStarted()

			endpoint := fmt.Sprintf("%s:%s", sub.Repository, sub.Subscription.Workflow)
			fe.logger.Debug("Starting child workflow execution",
				"repository", sub.Repository,
				"workflow", sub.Subscription.Workflow,
				"endpoint", endpoint,
			)

			// Update child status to running
			state.UpdateChildStatus(sub.Repository, sub.Subscription.Workflow, ChildStatusRunning, "", "")

			// Get circuit breaker for this endpoint
			circuitBreaker := fe.circuitBreakerManager.GetCircuitBreaker(endpoint)
			retryExecutor := NewRetryableExecutor(fe.retryConfig)

			var finalErr error
			var runID string

			// Execute with resilience (circuit breaker + retry)
			err := circuitBreaker.Call(func() error {
				return retryExecutor.ExecuteWithCallback(context.Background(), func() error {
					return fe.simulateWorkflowTrigger(sub.Repository, sub.Subscription.Workflow, childWorkflow.Inputs)
				}, func(attempt int, retryErr error) {
					fe.logger.Warn("Child workflow execution retry",
						"repository", sub.Repository,
						"workflow", sub.Subscription.Workflow,
						"attempt", attempt,
						"error", retryErr.Error(),
					)
				})
			})

			// Determine final status and record metrics
			var finalStatus ChildWorkflowStatus
			if err != nil {
				finalErr = err
				finalStatus = ChildStatusFailed
				if strings.Contains(err.Error(), "circuit breaker is open") {
					fe.logger.Warn("Child workflow blocked by circuit breaker",
						"repository", sub.Repository,
						"workflow", sub.Subscription.Workflow,
						"endpoint", endpoint,
					)
				}

				mutex.Lock()
				errors = append(errors, fmt.Sprintf("failed to trigger workflow in %s: %v", sub.Repository, err))
				mutex.Unlock()
			} else {
				finalStatus = ChildStatusCompleted
				runID = fmt.Sprintf("run-%d", time.Now().Unix())

				// Mark workflow as triggered for idempotency
				if markErr := state.MarkWorkflowTriggered(sub.Repository, sub.Subscription.Workflow, runID); markErr != nil {
					fe.logger.Warn("Failed to mark workflow as triggered",
						"repository", sub.Repository,
						"workflow", sub.Subscription.Workflow,
						"run_id", runID,
						"error", markErr.Error(),
					)
				}

				mutex.Lock()
				triggeredCount++
				mutex.Unlock()
			}

			// Record child completion metrics
			childDuration := time.Since(childStartTime)
			fe.metricsCollector.RecordChildCompleted(childDuration, finalStatus)

			// Update final child status
			state.UpdateChildStatus(sub.Repository, sub.Subscription.Workflow, finalStatus, runID,
				func() string {
					if finalErr != nil {
						return finalErr.Error()
					}
					return ""
				}())

			fe.logger.Info("Child workflow execution completed",
				"repository", sub.Repository,
				"workflow", sub.Subscription.Workflow,
				"status", finalStatus,
				"duration_ms", childDuration.Milliseconds(),
				"run_id", runID,
			)
		}(subscriber, child)
	}

	wg.Wait()
	return triggeredCount, errors
}

// simulateWorkflowTrigger simulates triggering a workflow in a repository.
// This is a placeholder for Phase 2 - actual workflow triggering will be implemented in later phases.
func (fe *FanOutExecutor) simulateWorkflowTrigger(repository, workflow string, inputs map[string]string) error {
	if fe.debug {
		fmt.Printf("SIMULATION: Would trigger workflow '%s' in '%s' with inputs: %v\n", workflow, repository, inputs)
	}

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	// For testing purposes, fail if repository name contains "fail"
	if strings.Contains(repository, "fail") {
		return fmt.Errorf("simulated failure for repository %s", repository)
	}

	return nil
}

// waitForChildrenWithState waits for child workflows to complete using state management.
func (fe *FanOutExecutor) waitForChildrenWithState(state *FanOutState, timeout time.Duration) error {
	if fe.debug {
		fmt.Printf("Waiting for children using state management\n")
	}

	// Set default timeout if not provided
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	// Poll for completion with exponential backoff
	pollInterval := 100 * time.Millisecond
	maxPollInterval := 1 * time.Second
	startTime := time.Now()

	for {
		// Check if timeout exceeded
		if time.Since(startTime) > timeout {
			state.TimeoutFanOut()
			return fmt.Errorf("timeout exceeded while waiting for children")
		}

		// Check if fan-out is complete
		if state.IsComplete() {
			if fe.debug {
				summary := state.GetSummary()
				if summary.FailedChildren > 0 || summary.TimedOutChildren > 0 {
					fmt.Printf("Children completed with failures: %d failed, %d timed out\n",
						summary.FailedChildren, summary.TimedOutChildren)
				} else {
					fmt.Printf("All children completed successfully\n")
				}
			}
			return nil
		}

		// Sleep before next poll
		time.Sleep(pollInterval)

		// Exponential backoff up to max interval
		if pollInterval < maxPollInterval {
			pollInterval = pollInterval * 2
			if pollInterval > maxPollInterval {
				pollInterval = maxPollInterval
			}
		}
	}
}

// waitForChildren waits for child workflows to complete (legacy method for backward compatibility).
// This is a simplified implementation - new code should use waitForChildrenWithState.
func (fe *FanOutExecutor) waitForChildren(subscribers []SubscriptionMatch, params *FanOutParams) error {
	if fe.debug {
		fmt.Printf("SIMULATION: Waiting for children (simplified implementation)\n")
	}

	// Parse timeout if provided
	var timeout time.Duration
	if params.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(params.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout format: %v", err)
		}
	} else {
		timeout = 5 * time.Minute // Default timeout
	}

	// Simulate waiting for children
	waitTime := time.Duration(len(subscribers)) * 50 * time.Millisecond
	if waitTime > timeout {
		return fmt.Errorf("timeout exceeded while waiting for children")
	}

	time.Sleep(waitTime)

	if fe.debug {
		fmt.Printf("All children completed (simulation)\n")
	}

	return nil
}

// convertPayload converts a string map to interface{} map for Event payload.
func convertPayload(stringPayload map[string]string) map[string]interface{} {
	payload := make(map[string]interface{})
	for key, value := range stringPayload {
		payload[key] = value
	}
	return payload
}

// GetMetrics returns current fan-out metrics.
func (fe *FanOutExecutor) GetMetrics() FanOutMetrics {
	return fe.metricsCollector.GetMetrics()
}

// GetHealthStatus returns the current health status.
func (fe *FanOutExecutor) GetHealthStatus() HealthStatus {
	return fe.healthChecker.CheckHealth()
}

// GetCircuitBreakerStats returns circuit breaker statistics for all endpoints.
func (fe *FanOutExecutor) GetCircuitBreakerStats() map[string]CircuitBreakerStats {
	return fe.circuitBreakerManager.GetAllStats()
}

// ResetMetrics resets all collected metrics.
func (fe *FanOutExecutor) ResetMetrics() {
	fe.metricsCollector.Reset()
}

// ResetCircuitBreakers resets all circuit breakers.
func (fe *FanOutExecutor) ResetCircuitBreakers() {
	fe.circuitBreakerManager.ResetAll()
}

// SetHealthThresholds allows customization of health check thresholds.
func (fe *FanOutExecutor) SetHealthThresholds(errorRate, latency float64, activeOps int64) {
	fe.healthChecker.SetThresholds(errorRate, latency, activeOps)
}

// ConfigureRetry allows runtime configuration of retry behavior.
func (fe *FanOutExecutor) ConfigureRetry(config RetryConfig) {
	fe.retryConfig = config
}

// ConfigureCircuitBreaker allows runtime configuration of circuit breaker behavior.
func (fe *FanOutExecutor) ConfigureCircuitBreaker(config CircuitBreakerConfig) {
	fe.circuitBreakerConfig = config
	// Note: This affects new circuit breakers only; existing ones retain their configuration
}
