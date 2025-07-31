// Package engine provides the core fan-out execution capabilities for Tako workflows.
//
// # Fan-Out with Idempotency
//
// The FanOutExecutor supports optional idempotency to prevent duplicate workflow executions
// when the same event is processed multiple times. This is particularly useful in distributed
// systems where events might be retried or replayed.
//
// Key Features:
//   - Deterministic event fingerprinting using SHA256 hashing
//   - Persistent state management across process restarts
//   - Configurable retention periods for idempotent states
//   - Atomic file operations to handle concurrent duplicates
//   - Backward compatible (disabled by default)
//
// Example Usage:
//
//	// Create executor with idempotency enabled
//	executor, err := NewFanOutExecutor("/cache/dir", false, workflowRunner)
//	if err != nil {
//	    return err
//	}
//	executor.SetIdempotency(true)
//
//	// Configure custom retention for idempotent states (optional)
//	executor.stateManager.SetIdempotencyRetention(48 * time.Hour)
//
//	// Execute fan-out step (duplicates will be detected automatically)
//	result, err := executor.Execute(step, sourceRepo)
//
// Duplicate Detection:
//   - Events with the same type, source, and payload produce identical fingerprints
//   - Fingerprints are used as state identifiers instead of timestamps
//   - Existing states are loaded and their results returned for duplicates
//   - Only the first execution triggers workflows; duplicates return cached results
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
	"github.com/dangazineu/tako/internal/interfaces"
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
	cleanupManager        *CleanupManager
	logger                Logger
	workflowRunner        interfaces.WorkflowRunner
	cacheDir              string
	debug                 bool

	// Configuration
	retryConfig          RetryConfig
	circuitBreakerConfig CircuitBreakerConfig
	enableIdempotency    bool
}

// NewFanOutExecutor creates a new fan-out executor.
func NewFanOutExecutor(cacheDir string, debug bool, workflowRunner interfaces.WorkflowRunner) (*FanOutExecutor, error) {
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
	cleanupManager := NewCleanupManager(filepath.Join(cacheDir, "workspaces"), 0, debug) // Use default maxAge
	logger := NewStructuredLogger(debug)

	return &FanOutExecutor{
		discoveryManager:      discoveryManager,
		subscriptionEvaluator: subscriptionEvaluator,
		stateManager:          stateManager,
		eventValidator:        eventValidator,
		circuitBreakerManager: circuitBreakerManager,
		metricsCollector:      metricsCollector,
		healthChecker:         healthChecker,
		cleanupManager:        cleanupManager,
		logger:                logger,
		workflowRunner:        workflowRunner,
		cacheDir:              cacheDir,
		debug:                 debug,
		retryConfig:           retryConfig,
		circuitBreakerConfig:  circuitBreakerConfig,
		enableIdempotency:     false, // Default to disabled for backward compatibility
	}, nil
}

// SetIdempotency enables or disables idempotency checking for duplicate events.
//
// When enabled, the executor will prevent duplicate workflow executions for the same event
// by using deterministic event fingerprinting and persistent state management.
//
// How Idempotency Works:
//  1. Each event gets a deterministic fingerprint based on type, source, and payload
//  2. The executor checks for existing states with the same fingerprint
//  3. If found, returns cached result instead of triggering new workflows
//  4. If not found, proceeds with normal execution and saves state for future duplicates
//
// Benefits:
//   - Prevents duplicate workflow executions during retries or system restarts
//   - Maintains consistency across distributed systems
//   - Reduces resource usage and improves reliability
//
// Usage Examples:
//
//	// Enable idempotency for production deployments
//	executor.SetIdempotency(true)
//
//	// Disable for testing or when duplicates are desired
//	executor.SetIdempotency(false)
//
// Configuration Notes:
//   - Idempotency is disabled by default for backward compatibility
//   - When enabled, requires additional disk space for state persistence
//   - Idempotent states are retained for 24 hours by default (configurable)
//   - Works across process restarts and multiple executor instances
//
// Performance Impact:
//   - Minimal overhead for fingerprint generation (~microseconds)
//   - Small disk I/O overhead for state persistence
//   - Significant savings when duplicates are prevented
func (fe *FanOutExecutor) SetIdempotency(enabled bool) {
	fe.enableIdempotency = enabled
}

// IsIdempotencyEnabled returns whether idempotency checking is enabled.
func (fe *FanOutExecutor) IsIdempotencyEnabled() bool {
	return fe.enableIdempotency
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

// ChildExecutionError represents detailed error information for a child workflow execution.
type ChildExecutionError struct {
	Repository   string        `json:"repository"`
	Workflow     string        `json:"workflow"`
	RunID        string        `json:"run_id,omitempty"`
	ErrorType    string        `json:"error_type"` // "execution_failed", "workflow_failed", "timeout", "circuit_breaker"
	ErrorMessage string        `json:"error_message"`
	StartTime    time.Time     `json:"start_time"`
	Duration     time.Duration `json:"duration"`
	RetryCount   int           `json:"retry_count"`
}

// FanOutResult represents the result of a fan-out execution.
type FanOutResult struct {
	Success          bool
	EventEmitted     bool
	SubscribersFound int
	TriggeredCount   int
	Errors           []string              // Legacy simple error messages
	DetailedErrors   []ChildExecutionError // Detailed error information
	StartTime        time.Time
	EndTime          time.Time
	FanOutID         string         // ID of the fan-out state for tracking
	TimeoutExceeded  bool           // Whether the overall operation timed out
	ChildrenSummary  *FanOutSummary // Summary of child workflow statuses
}

// Execute performs the fan-out operation with proper state management.
func (fe *FanOutExecutor) Execute(step config.WorkflowStep, sourceRepo string) (*FanOutResult, error) {
	return fe.ExecuteWithContext(step, sourceRepo, "")
}

// ExecuteWithSubscriptions performs the fan-out operation with pre-discovered subscriptions.
func (fe *FanOutExecutor) ExecuteWithSubscriptions(step config.WorkflowStep, sourceRepo string, subscriptions []interfaces.SubscriptionMatch) (*FanOutResult, error) {
	return fe.executeWithContextAndSubscriptions(step, sourceRepo, "", subscriptions)
}

// ExecuteWithContext performs the fan-out operation with optional parent run context.
func (fe *FanOutExecutor) ExecuteWithContext(step config.WorkflowStep, sourceRepo, parentRunID string) (*FanOutResult, error) {
	// Backward compatibility - discover subscriptions internally
	return fe.executeWithContextAndSubscriptions(step, sourceRepo, parentRunID, nil)
}

// executeWithContextAndSubscriptions is the internal implementation that optionally accepts pre-discovered subscriptions.
func (fe *FanOutExecutor) executeWithContextAndSubscriptions(step config.WorkflowStep, sourceRepo, parentRunID string, preDiscoveredSubscriptions []interfaces.SubscriptionMatch) (*FanOutResult, error) {
	startTime := time.Now()
	result := &FanOutResult{
		StartTime:       startTime,
		Errors:          []string{},
		DetailedErrors:  []ChildExecutionError{},
		TimeoutExceeded: false,
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

	var timeout time.Duration
	if params.Timeout != "" {
		timeout, err = time.ParseDuration(params.Timeout)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("invalid timeout format: %v", err))
			result.EndTime = time.Now()
			return result, err
		}
	}

	// Check for idempotency and handle duplicate events
	var state *FanOutState
	var fanOutID string
	var eventFingerprint string

	if fe.enableIdempotency {
		// Create enhanced event from parameters for fingerprinting
		// Note: We DON'T use EventBuilder here because it generates unique IDs,
		// which would defeat the purpose of idempotency. Instead, we create the event
		// manually without an ID so fingerprinting falls back to payload hashing.
		enhancedEvent := EnhancedEvent{
			Type:    params.EventType,
			Payload: params.Payload,
			Metadata: EventMetadata{
				Source:  sourceRepo,
				Headers: make(map[string]string),
				// Note: No ID or Timestamp set - this makes fingerprinting deterministic
			},
		}

		// Set schema if provided
		if params.SchemaVersion != "" {
			enhancedEvent.Schema = fmt.Sprintf("%s@%s", params.EventType, params.SchemaVersion)
		}

		// Generate event fingerprint
		eventFingerprint, err = GenerateEventFingerprint(&enhancedEvent)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to generate event fingerprint: %v", err))
			result.EndTime = time.Now()
			return result, err
		}

		if fe.debug {
			fmt.Printf("Generated event fingerprint: %s for event '%s' from '%s'\n", eventFingerprint, params.EventType, sourceRepo)
		}

		// Check for existing state with same fingerprint
		existingState, err := fe.stateManager.GetFanOutStateByFingerprint(eventFingerprint)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to check for existing state: %v", err))
			result.EndTime = time.Now()
			return result, err
		}

		if existingState != nil {
			if fe.debug {
				fmt.Printf("Found existing state for fingerprint %s: %s (status: %s)\n", eventFingerprint, existingState.ID, existingState.Status)
			}

			// Handle duplicate event based on existing state status
			return fe.handleDuplicateEvent(existingState, timeout, startTime)
		}

		// No duplicate found, create new state with fingerprint
		fanOutID = fmt.Sprintf("fanout-%s", eventFingerprint)
		result.FanOutID = fanOutID

		state, err = fe.stateManager.CreateFanOutStateWithFingerprint(fanOutID, eventFingerprint, parentRunID, sourceRepo, params.EventType, params.WaitForChildren, timeout) //nolint:staticcheck,ineffassign
	} else {
		// Traditional creation without idempotency - use nanoseconds for uniqueness
		fanOutID = fmt.Sprintf("fanout-%d-%s", startTime.UnixNano(), params.EventType)
		result.FanOutID = fanOutID

		state, err = fe.stateManager.CreateFanOutState(fanOutID, parentRunID, sourceRepo, params.EventType, params.WaitForChildren, timeout)
	}
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

	// Use pre-discovered subscriptions if provided, otherwise discover them
	var subscribers []interfaces.SubscriptionMatch
	if preDiscoveredSubscriptions != nil {
		// Use the pre-discovered subscriptions
		subscribers = preDiscoveredSubscriptions
		if fe.debug {
			fmt.Printf("Using %d pre-discovered subscriptions\n", len(subscribers))
		}
	} else {
		// Find subscribers for this event (backward compatibility)
		artifact := fmt.Sprintf("%s:default", sourceRepo)
		discoveredSubscribers, err := fe.discoveryManager.FindSubscribers(artifact, params.EventType)
		if err != nil {
			state.FailFanOut(fmt.Sprintf("failed to find subscribers: %v", err))
			result.Errors = append(result.Errors, fmt.Sprintf("failed to find subscribers: %v", err))
			result.EndTime = time.Now()
			return result, err
		}
		subscribers = discoveredSubscribers
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
		triggeredCount, errors, detailedErrors := fe.triggerSubscribersWithState(validSubscribers, event, params, state)
		result.TriggeredCount = triggeredCount
		result.Errors = append(result.Errors, errors...)
		result.DetailedErrors = append(result.DetailedErrors, detailedErrors...)
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

	// Get final children summary
	summary := state.GetSummary()
	result.ChildrenSummary = &summary

	// Determine if operation timed out
	if result.ChildrenSummary != nil && result.ChildrenSummary.TimedOutChildren > 0 {
		result.TimeoutExceeded = true
	}

	result.Success = len(result.Errors) == 0
	result.EndTime = time.Now()

	if fe.debug {
		fmt.Printf("Fan-out completed: success=%v, triggered=%d, errors=%d, detailed_errors=%d\n",
			result.Success, result.TriggeredCount, len(result.Errors), len(result.DetailedErrors))
		if result.ChildrenSummary != nil {
			fmt.Printf("Children summary: total=%d, completed=%d, failed=%d, timed_out=%d\n",
				result.ChildrenSummary.TotalChildren, result.ChildrenSummary.CompletedChildren,
				result.ChildrenSummary.FailedChildren, result.ChildrenSummary.TimedOutChildren)
		}
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
func (fe *FanOutExecutor) triggerSubscribersWithState(subscribers []SubscriptionMatch, event Event, params *FanOutParams, state *FanOutState) (int, []string, []ChildExecutionError) {
	errors := []string{}
	detailedErrors := []ChildExecutionError{}
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
			var executionResult *interfaces.ExecutionResult
			var retryCount int

			// Create context with timeout for child execution
			ctx := context.Background()
			if params.Timeout != "" {
				if timeout, parseErr := time.ParseDuration(params.Timeout); parseErr == nil {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, timeout)
					defer cancel()
				}
			}

			// Execute with resilience (circuit breaker + retry)
			err := circuitBreaker.Call(func() error {
				return retryExecutor.ExecuteWithCallback(ctx, func() error {
					result, execErr := fe.executeChildWorkflow(ctx, sub.Repository, sub.Subscription.Workflow, childWorkflow.Inputs)
					if execErr != nil {
						return execErr
					}
					// Store the result for later use
					executionResult = result
					if result != nil {
						runID = result.RunID
					}
					return nil
				}, func(attempt int, retryErr error) {
					retryCount = attempt
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
			childDuration := time.Since(childStartTime)

			if err != nil {
				finalErr = err
				finalStatus = ChildStatusFailed

				// Determine error type for detailed reporting
				var errorType string
				if strings.Contains(err.Error(), "circuit breaker is open") {
					errorType = "circuit_breaker"
					fe.logger.Warn("Child workflow blocked by circuit breaker",
						"repository", sub.Repository,
						"workflow", sub.Subscription.Workflow,
						"endpoint", endpoint,
					)
				} else if strings.Contains(err.Error(), "context deadline exceeded") {
					errorType = "timeout"
					finalStatus = ChildStatusTimedOut
				} else {
					errorType = "execution_failed"
				}

				mutex.Lock()
				errors = append(errors, fmt.Sprintf("failed to trigger workflow in %s: %v", sub.Repository, err))
				detailedErrors = append(detailedErrors, ChildExecutionError{
					Repository:   sub.Repository,
					Workflow:     sub.Subscription.Workflow,
					RunID:        runID,
					ErrorType:    errorType,
					ErrorMessage: err.Error(),
					StartTime:    childStartTime,
					Duration:     childDuration,
					RetryCount:   retryCount,
				})
				mutex.Unlock()
			} else {
				// Execution completed, but check if the workflow itself succeeded
				if executionResult != nil && !executionResult.Success {
					finalStatus = ChildStatusFailed
					finalErr = fmt.Errorf("child workflow execution completed but workflow failed")

					mutex.Lock()
					errors = append(errors, fmt.Sprintf("workflow failed in %s: workflow execution was unsuccessful", sub.Repository))
					detailedErrors = append(detailedErrors, ChildExecutionError{
						Repository:   sub.Repository,
						Workflow:     sub.Subscription.Workflow,
						RunID:        runID,
						ErrorType:    "workflow_failed",
						ErrorMessage: "child workflow execution was unsuccessful",
						StartTime:    childStartTime,
						Duration:     childDuration,
						RetryCount:   retryCount,
					})
					mutex.Unlock()
				} else {
					finalStatus = ChildStatusCompleted
					// runID is already set from the execution result

					// Schedule cleanup of child workspace (async, best effort)
					if runID != "" {
						go func(cleanupRunID string) {
							if cleanupErr := fe.cleanupManager.CleanupChildWorkspace(cleanupRunID); cleanupErr != nil && fe.debug {
								fmt.Printf("Warning: Failed to cleanup child workspace for runID %s: %v\n", cleanupRunID, cleanupErr)
							}
						}(runID)
					}

					mutex.Lock()
					triggeredCount++
					mutex.Unlock()
				}
			}

			// Record child completion metrics
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
	return triggeredCount, errors, detailedErrors
}

// executeChildWorkflow executes a workflow in a child repository using the injected WorkflowRunner.
// This replaces the simulation with actual isolated child workflow execution.
func (fe *FanOutExecutor) executeChildWorkflow(ctx context.Context, repository, workflow string, inputs map[string]string) (*interfaces.ExecutionResult, error) {
	if fe.workflowRunner == nil {
		return nil, fmt.Errorf("workflow runner not configured for child execution")
	}

	if fe.debug {
		fmt.Printf("EXECUTING: Triggering workflow '%s' in '%s' with inputs: %v\n", workflow, repository, inputs)
	}

	// Execute the child workflow using the injected WorkflowRunner
	result, err := fe.workflowRunner.ExecuteWorkflow(ctx, repository, workflow, inputs)
	if err != nil {
		return nil, fmt.Errorf("child workflow execution failed in %s: %w", repository, err)
	}

	if fe.debug {
		status := "SUCCESS"
		if result != nil && !result.Success {
			status = "FAILED"
		}
		fmt.Printf("COMPLETED: Child workflow '%s' in '%s' - Status: %s\n", workflow, repository, status)
	}

	return result, nil
}

// handleDuplicateEvent handles different scenarios when a duplicate event is detected.
func (fe *FanOutExecutor) handleDuplicateEvent(existingState *FanOutState, timeout time.Duration, startTime time.Time) (*FanOutResult, error) {
	switch existingState.Status {
	case FanOutStatusCompleted, FanOutStatusFailed, FanOutStatusTimedOut:
		// State is complete, reconstruct and return result
		if fe.debug {
			fmt.Printf("Duplicate event detected: state %s is already complete (%s)\n", existingState.ID, existingState.Status)
		}
		return fe.reconstructFanOutResult(existingState, startTime), nil

	case FanOutStatusRunning, FanOutStatusWaiting:
		// State is still running, wait for completion
		if fe.debug {
			fmt.Printf("Duplicate event detected: state %s is still running (%s), waiting for completion\n", existingState.ID, existingState.Status)
		}
		return fe.waitForExistingState(existingState, timeout, startTime)

	default:
		// Pending state - treat as running and wait
		if fe.debug {
			fmt.Printf("Duplicate event detected: state %s is pending, waiting for completion\n", existingState.ID)
		}
		return fe.waitForExistingState(existingState, timeout, startTime)
	}
}

// reconstructFanOutResult creates a FanOutResult from an existing FanOutState.
func (fe *FanOutExecutor) reconstructFanOutResult(state *FanOutState, startTime time.Time) *FanOutResult {
	summary := state.GetSummary()

	result := &FanOutResult{
		Success:          state.Status == FanOutStatusCompleted,
		EventEmitted:     true, // Event was emitted in the original execution
		SubscribersFound: summary.TotalChildren,
		TriggeredCount:   0, // Duplicate call - no new workflows were triggered
		Errors:           []string{},
		DetailedErrors:   []ChildExecutionError{},
		StartTime:        startTime,  // Use current call's start time
		EndTime:          time.Now(), // End time is now for the duplicate call
		FanOutID:         state.ID,
		TimeoutExceeded:  summary.TimedOutChildren > 0,
		ChildrenSummary:  &summary,
	}

	// Add error message if the original execution failed
	if state.Status == FanOutStatusFailed && state.ErrorMessage != "" {
		result.Errors = append(result.Errors, fmt.Sprintf("original execution failed: %s", state.ErrorMessage))
	}

	// Add summary errors for failed children
	if summary.FailedChildren > 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("%d child workflows failed", summary.FailedChildren))
	}
	if summary.TimedOutChildren > 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("%d child workflows timed out", summary.TimedOutChildren))
	}

	return result
}

// waitForExistingState waits for an existing state to complete and returns the result.
func (fe *FanOutExecutor) waitForExistingState(state *FanOutState, timeout time.Duration, startTime time.Time) (*FanOutResult, error) {
	// Use the original timeout or a reasonable default
	waitTimeout := timeout
	if waitTimeout == 0 {
		waitTimeout = 5 * time.Minute
	}

	// Poll for completion
	pollInterval := 100 * time.Millisecond
	maxPollInterval := 1 * time.Second
	waitStartTime := time.Now()

	for {
		// Check if timeout exceeded
		if time.Since(waitStartTime) > waitTimeout {
			// Reconstruct result with timeout indication
			result := fe.reconstructFanOutResult(state, startTime)
			result.TimeoutExceeded = true
			result.Errors = append(result.Errors, "timeout exceeded while waiting for existing execution to complete")
			return result, nil
		}

		// Check if state is complete
		if state.IsComplete() {
			return fe.reconstructFanOutResult(state, startTime), nil
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

		// Refresh state from disk/memory to get latest status
		freshState, err := fe.stateManager.GetFanOutState(state.ID)
		if err != nil {
			// If we can't refresh the state, return current result
			if fe.debug {
				fmt.Printf("Warning: failed to refresh state %s: %v\n", state.ID, err)
			}
			return fe.reconstructFanOutResult(state, startTime), nil
		}
		state = freshState
	}
}

// simulateWorkflowTrigger is kept for backward compatibility with tests.
// TODO: Remove this method after all tests are updated to use real execution.
func (fe *FanOutExecutor) simulateWorkflowTrigger(repository, workflow string, inputs map[string]string) error {
	// Convert to real execution with a background context
	_, err := fe.executeChildWorkflow(context.Background(), repository, workflow, inputs)
	return err
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

// CleanupOrphanedWorkspaces removes orphaned child workflow workspaces.
func (fe *FanOutExecutor) CleanupOrphanedWorkspaces() error {
	return fe.cleanupManager.CleanupOrphanedWorkspaces()
}

// GetOrphanedWorkspaceStats returns statistics about orphaned workspaces.
func (fe *FanOutExecutor) GetOrphanedWorkspaceStats() (int, int64, error) {
	return fe.cleanupManager.GetOrphanedWorkspaceStats()
}
