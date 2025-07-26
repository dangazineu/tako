package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ExecutionStatus represents the current status of an execution
type ExecutionStatus string

const (
	StatusPending   ExecutionStatus = "pending"
	StatusRunning   ExecutionStatus = "running"
	StatusCompleted ExecutionStatus = "completed"
	StatusFailed    ExecutionStatus = "failed"
	StatusCancelled ExecutionStatus = "cancelled"
)

// ExecutionState manages the persistent state of workflow executions
// Supports hierarchical state management for multi-repository execution trees
type ExecutionState struct {
	RunID        string            `json:"run_id"`
	Status       ExecutionStatus   `json:"status"`
	WorkflowName string            `json:"workflow_name"`
	Repository   string            `json:"repository"`
	Inputs       map[string]string `json:"inputs"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      *time.Time        `json:"end_time,omitempty"`
	Error        string            `json:"error,omitempty"`

	// Execution tree support
	ParentRunID string   `json:"parent_run_id,omitempty"`
	ChildRuns   []string `json:"child_runs,omitempty"`

	// Step-level state
	Steps       map[string]*StepState `json:"steps"`
	CurrentStep string                `json:"current_step,omitempty"`

	// Metadata
	Version     string    `json:"version"`
	LastUpdated time.Time `json:"last_updated"`

	// Internal state management
	stateFile string
	mu        sync.RWMutex
}

// StepState represents the state of an individual workflow step
type StepState struct {
	ID         string            `json:"id"`
	Status     ExecutionStatus   `json:"status"`
	StartTime  *time.Time        `json:"start_time,omitempty"`
	EndTime    *time.Time        `json:"end_time,omitempty"`
	Error      string            `json:"error,omitempty"`
	Output     string            `json:"output,omitempty"`
	Outputs    map[string]string `json:"outputs,omitempty"`
	RetryCount int               `json:"retry_count"`
}

// NewExecutionState creates a new execution state manager
func NewExecutionState(runID, workspaceRoot string) (*ExecutionState, error) {
	stateDir := filepath.Join(workspaceRoot, "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %v", err)
	}

	stateFile := filepath.Join(stateDir, "execution.json")

	state := &ExecutionState{
		RunID:       runID,
		Status:      StatusPending,
		Steps:       make(map[string]*StepState),
		Version:     "1.0",
		LastUpdated: time.Now(),
		stateFile:   stateFile,
	}

	// Try to load existing state
	if err := state.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load existing state: %v", err)
	}

	return state, nil
}

// LoadExecutionState loads an existing execution state from disk
func LoadExecutionState(runID, workspaceRoot string) (*ExecutionState, error) {
	stateFile := filepath.Join(workspaceRoot, "state", "execution.json")

	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %v", err)
	}

	var state ExecutionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %v", err)
	}

	state.stateFile = stateFile

	// Validate run ID matches
	if state.RunID != runID {
		return nil, fmt.Errorf("run ID mismatch: expected %s, got %s", runID, state.RunID)
	}

	return &state, nil
}

// StartExecution marks the beginning of workflow execution
func (s *ExecutionState) StartExecution(workflowName, repository string, inputs map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Status = StatusRunning
	s.WorkflowName = workflowName
	s.Repository = repository
	s.Inputs = inputs
	s.StartTime = time.Now()
	s.LastUpdated = time.Now()

	return s.save()
}

// CompleteExecution marks the successful completion of workflow execution
func (s *ExecutionState) CompleteExecution() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.Status = StatusCompleted
	s.EndTime = &now
	s.LastUpdated = now
	s.CurrentStep = ""

	return s.save()
}

// FailExecution marks the execution as failed with an error message
func (s *ExecutionState) FailExecution(errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.Status = StatusFailed
	s.EndTime = &now
	s.Error = errorMsg
	s.LastUpdated = now

	return s.save()
}

// CancelExecution marks the execution as cancelled
func (s *ExecutionState) CancelExecution() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.Status = StatusCancelled
	s.EndTime = &now
	s.LastUpdated = now

	return s.save()
}

// StartStep marks the beginning of a workflow step execution
func (s *ExecutionState) StartStep(stepID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	if s.Steps[stepID] == nil {
		s.Steps[stepID] = &StepState{
			ID:        stepID,
			Status:    StatusRunning,
			StartTime: &now,
			Outputs:   make(map[string]string),
		}
	} else {
		// Update existing step
		s.Steps[stepID].Status = StatusRunning
		s.Steps[stepID].StartTime = &now
		s.Steps[stepID].RetryCount++
	}

	s.CurrentStep = stepID
	s.LastUpdated = now

	return s.save()
}

// CompleteStep marks a step as successfully completed
func (s *ExecutionState) CompleteStep(stepID, output string, outputs map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	step := s.Steps[stepID]
	if step == nil {
		return fmt.Errorf("step %s not found", stepID)
	}

	now := time.Now()
	step.Status = StatusCompleted
	step.EndTime = &now
	step.Output = output
	if outputs != nil {
		step.Outputs = outputs
	}

	s.LastUpdated = now

	return s.save()
}

// FailStep marks a step as failed with an error message
func (s *ExecutionState) FailStep(stepID, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	step := s.Steps[stepID]
	if step == nil {
		return fmt.Errorf("step %s not found", stepID)
	}

	now := time.Now()
	step.Status = StatusFailed
	step.EndTime = &now
	step.Error = errorMsg

	s.LastUpdated = now

	return s.save()
}

// AddChildRun adds a child run ID to the execution tree
func (s *ExecutionState) AddChildRun(childRunID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ChildRuns = append(s.ChildRuns, childRunID)
	s.LastUpdated = time.Now()

	return s.save()
}

// GetStatus returns the current execution status (thread-safe)
func (s *ExecutionState) GetStatus() ExecutionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status
}

// GetStepStatus returns the status of a specific step (thread-safe)
func (s *ExecutionState) GetStepStatus(stepID string) ExecutionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if step := s.Steps[stepID]; step != nil {
		return step.Status
	}
	return StatusPending
}

// GetFailedSteps returns a list of failed steps for resume operations
func (s *ExecutionState) GetFailedSteps() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var failedSteps []string
	for stepID, step := range s.Steps {
		if step.Status == StatusFailed {
			failedSteps = append(failedSteps, stepID)
		}
	}

	return failedSteps
}

// GetCompletedSteps returns a list of successfully completed steps
func (s *ExecutionState) GetCompletedSteps() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var completedSteps []string
	for stepID, step := range s.Steps {
		if step.Status == StatusCompleted {
			completedSteps = append(completedSteps, stepID)
		}
	}

	return completedSteps
}

// GetStepOutputs returns the outputs of a specific step
func (s *ExecutionState) GetStepOutputs(stepID string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if step := s.Steps[stepID]; step != nil {
		return step.Outputs
	}
	return nil
}

// IsResumable returns true if the execution can be resumed
func (s *ExecutionState) IsResumable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Status == StatusFailed && len(s.GetFailedSteps()) > 0
}

// GetExecutionSummary returns a summary of the execution state
func (s *ExecutionState) GetExecutionSummary() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary := map[string]interface{}{
		"run_id":        s.RunID,
		"status":        s.Status,
		"workflow_name": s.WorkflowName,
		"repository":    s.Repository,
		"start_time":    s.StartTime,
		"last_updated":  s.LastUpdated,
	}

	if s.EndTime != nil {
		summary["end_time"] = *s.EndTime
		summary["duration"] = s.EndTime.Sub(s.StartTime).String()
	}

	if s.Error != "" {
		summary["error"] = s.Error
	}

	// Step statistics
	var pending, running, completed, failed int
	for _, step := range s.Steps {
		switch step.Status {
		case StatusPending:
			pending++
		case StatusRunning:
			running++
		case StatusCompleted:
			completed++
		case StatusFailed:
			failed++
		}
	}

	summary["steps"] = map[string]int{
		"total":     len(s.Steps),
		"pending":   pending,
		"running":   running,
		"completed": completed,
		"failed":    failed,
	}

	return summary
}

// save persists the execution state to disk
func (s *ExecutionState) save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %v", err)
	}

	// Write to temporary file first, then atomic rename
	tempFile := s.stateFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %v", err)
	}

	if err := os.Rename(tempFile, s.stateFile); err != nil {
		os.Remove(tempFile) // Clean up on failure
		return fmt.Errorf("failed to rename temp state file: %v", err)
	}

	return nil
}

// load loads the execution state from disk
func (s *ExecutionState) load() error {
	data, err := os.ReadFile(s.stateFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, s)
}
