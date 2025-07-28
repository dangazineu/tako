package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FanOutState represents the state of a fan-out operation and its child workflows.
type FanOutState struct {
	ID                 string                    `json:"id"`
	ParentRunID        string                    `json:"parent_run_id,omitempty"`
	SourceRepo         string                    `json:"source_repo"`
	EventType          string                    `json:"event_type"`
	Status             FanOutStatus              `json:"status"`
	StartTime          time.Time                 `json:"start_time"`
	EndTime            *time.Time                `json:"end_time,omitempty"`
	Children           map[string]*ChildWorkflow `json:"children"`
	WaitingForAll      bool                      `json:"waiting_for_all"`
	Timeout            time.Duration             `json:"timeout,omitempty"`
	ErrorMessage       string                    `json:"error_message,omitempty"`
	TriggeredWorkflows map[string]string         `json:"triggered_workflows"` // Key: repository/workflow, Value: runID

	// Runtime fields (not serialized)
	mu           sync.RWMutex        `json:"-"`
	stateManager *FanOutStateManager `json:"-"`
}

// ChildWorkflow represents a child workflow triggered by fan-out.
type ChildWorkflow struct {
	Repository   string              `json:"repository"`
	Workflow     string              `json:"workflow"`
	RunID        string              `json:"run_id,omitempty"`
	Status       ChildWorkflowStatus `json:"status"`
	StartTime    time.Time           `json:"start_time"`
	EndTime      *time.Time          `json:"end_time,omitempty"`
	ErrorMessage string              `json:"error_message,omitempty"`
	Inputs       map[string]string   `json:"inputs"`
}

// FanOutStatus represents the status of a fan-out operation.
type FanOutStatus string

const (
	FanOutStatusPending   FanOutStatus = "pending"
	FanOutStatusRunning   FanOutStatus = "running"
	FanOutStatusWaiting   FanOutStatus = "waiting"
	FanOutStatusCompleted FanOutStatus = "completed"
	FanOutStatusFailed    FanOutStatus = "failed"
	FanOutStatusTimedOut  FanOutStatus = "timed_out"
)

// ChildWorkflowStatus represents the status of a child workflow.
type ChildWorkflowStatus string

const (
	ChildStatusPending   ChildWorkflowStatus = "pending"
	ChildStatusRunning   ChildWorkflowStatus = "running"
	ChildStatusCompleted ChildWorkflowStatus = "completed"
	ChildStatusFailed    ChildWorkflowStatus = "failed"
	ChildStatusTimedOut  ChildWorkflowStatus = "timed_out"
)

// FanOutStateManager manages the persistent state of fan-out operations.
type FanOutStateManager struct {
	stateDir string
	mu       sync.RWMutex
	states   map[string]*FanOutState
}

// NewFanOutStateManager creates a new state manager for fan-out operations.
func NewFanOutStateManager(stateDir string) (*FanOutStateManager, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %v", err)
	}

	manager := &FanOutStateManager{
		stateDir: stateDir,
		states:   make(map[string]*FanOutState),
	}

	// Load existing states from disk
	if err := manager.loadStates(); err != nil {
		return nil, fmt.Errorf("failed to load existing states: %v", err)
	}

	return manager, nil
}

// CreateFanOutState creates a new fan-out state and persists it.
func (sm *FanOutStateManager) CreateFanOutState(id, parentRunID, sourceRepo, eventType string, waitingForAll bool, timeout time.Duration) (*FanOutState, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	state := &FanOutState{
		ID:                 id,
		ParentRunID:        parentRunID,
		SourceRepo:         sourceRepo,
		EventType:          eventType,
		Status:             FanOutStatusPending,
		StartTime:          time.Now(),
		Children:           make(map[string]*ChildWorkflow),
		WaitingForAll:      waitingForAll,
		Timeout:            timeout,
		TriggeredWorkflows: make(map[string]string),
		stateManager:       sm,
	}

	sm.states[id] = state

	data, err := state.persist()
	if err != nil {
		delete(sm.states, id)
		return nil, err
	}
	if err := sm.persistState(state.ID, data); err != nil {
		delete(sm.states, id)
		return nil, err
	}

	return state, nil
}

// GetFanOutState retrieves a fan-out state by ID.
func (sm *FanOutStateManager) GetFanOutState(id string) (*FanOutState, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	state, exists := sm.states[id]
	if !exists {
		return nil, fmt.Errorf("fan-out state not found: %s", id)
	}

	return state, nil
}

// AddChildWorkflow adds a child workflow to the fan-out state.
func (state *FanOutState) AddChildWorkflow(repository, workflow string, inputs map[string]string) (*ChildWorkflow, error) {
	childID := fmt.Sprintf("%s-%s", repository, workflow)
	child := &ChildWorkflow{
		Repository: repository,
		Workflow:   workflow,
		Status:     ChildStatusPending,
		StartTime:  time.Now(),
		Inputs:     inputs,
	}

	state.mu.Lock()
	state.Children[childID] = child
	data, err := state.persist()
	state.mu.Unlock()

	if err != nil {
		return nil, err
	}

	// Persist state after releasing lock
	if err := state.stateManager.persistState(state.ID, data); err != nil {
		return nil, err
	}

	return child, nil
}

// UpdateChildStatus updates the status of a child workflow.
func (state *FanOutState) UpdateChildStatus(repository, workflow string, status ChildWorkflowStatus, runID, errorMessage string) error {
	childID := fmt.Sprintf("%s-%s", repository, workflow)

	state.mu.Lock()
	child, exists := state.Children[childID]
	if !exists {
		state.mu.Unlock()
		return fmt.Errorf("child workflow not found: %s", childID)
	}

	child.Status = status
	if runID != "" {
		child.RunID = runID
	}
	if errorMessage != "" {
		child.ErrorMessage = errorMessage
	}
	if status == ChildStatusCompleted || status == ChildStatusFailed || status == ChildStatusTimedOut {
		now := time.Now()
		child.EndTime = &now
	}

	// Check if all children are complete and update parent status
	state.checkAndUpdateStatus()
	data, err := state.persist()
	state.mu.Unlock()

	if err != nil {
		return err
	}

	// Persist state after releasing lock
	return state.stateManager.persistState(state.ID, data)
}

// StartFanOut marks the fan-out as running.
func (state *FanOutState) StartFanOut() error {
	state.mu.Lock()
	state.Status = FanOutStatusRunning
	data, err := state.persist()
	state.mu.Unlock()

	if err != nil {
		return err
	}

	return state.stateManager.persistState(state.ID, data)
}

// StartWaiting marks the fan-out as waiting for children to complete.
func (state *FanOutState) StartWaiting() error {
	state.mu.Lock()
	if len(state.Children) == 0 {
		// No children to wait for, complete immediately
		state.Status = FanOutStatusCompleted
		now := time.Now()
		state.EndTime = &now
	} else {
		state.Status = FanOutStatusWaiting
		// Check if all children are already complete
		state.checkAndUpdateStatus()
	}
	data, err := state.persist()
	state.mu.Unlock()

	if err != nil {
		return err
	}

	return state.stateManager.persistState(state.ID, data)
}

// CompleteFanOut marks the fan-out as completed.
func (state *FanOutState) CompleteFanOut() error {
	state.mu.Lock()
	state.Status = FanOutStatusCompleted
	now := time.Now()
	state.EndTime = &now
	data, err := state.persist()
	state.mu.Unlock()

	if err != nil {
		return err
	}

	return state.stateManager.persistState(state.ID, data)
}

// FailFanOut marks the fan-out as failed.
func (state *FanOutState) FailFanOut(errorMessage string) error {
	state.mu.Lock()
	state.Status = FanOutStatusFailed
	state.ErrorMessage = errorMessage
	now := time.Now()
	state.EndTime = &now
	data, err := state.persist()
	state.mu.Unlock()

	if err != nil {
		return err
	}

	return state.stateManager.persistState(state.ID, data)
}

// TimeoutFanOut marks the fan-out as timed out.
func (state *FanOutState) TimeoutFanOut() error {
	state.mu.Lock()
	state.Status = FanOutStatusTimedOut
	now := time.Now()
	state.EndTime = &now
	data, err := state.persist()
	state.mu.Unlock()

	if err != nil {
		return err
	}

	return state.stateManager.persistState(state.ID, data)
}

// IsComplete returns true if the fan-out operation is complete (success, failure, or timeout).
func (state *FanOutState) IsComplete() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.Status == FanOutStatusCompleted ||
		state.Status == FanOutStatusFailed ||
		state.Status == FanOutStatusTimedOut
}

// GetSummary returns a summary of the fan-out state.
func (state *FanOutState) GetSummary() FanOutSummary {
	state.mu.RLock()
	defer state.mu.RUnlock()

	summary := FanOutSummary{
		ID:            state.ID,
		Status:        state.Status,
		StartTime:     state.StartTime,
		EndTime:       state.EndTime,
		TotalChildren: len(state.Children),
		ErrorMessage:  state.ErrorMessage,
	}

	for _, child := range state.Children {
		switch child.Status {
		case ChildStatusCompleted:
			summary.CompletedChildren++
		case ChildStatusFailed:
			summary.FailedChildren++
		case ChildStatusTimedOut:
			summary.TimedOutChildren++
		case ChildStatusRunning:
			summary.RunningChildren++
		case ChildStatusPending:
			summary.PendingChildren++
		}
	}

	return summary
}

// FanOutSummary provides a summary view of fan-out state.
type FanOutSummary struct {
	ID                string       `json:"id"`
	Status            FanOutStatus `json:"status"`
	StartTime         time.Time    `json:"start_time"`
	EndTime           *time.Time   `json:"end_time,omitempty"`
	TotalChildren     int          `json:"total_children"`
	CompletedChildren int          `json:"completed_children"`
	FailedChildren    int          `json:"failed_children"`
	TimedOutChildren  int          `json:"timed_out_children"`
	RunningChildren   int          `json:"running_children"`
	PendingChildren   int          `json:"pending_children"`
	ErrorMessage      string       `json:"error_message,omitempty"`
}

// checkAndUpdateStatus checks if all children are complete and updates the parent status accordingly.
// Must be called with state.mu held.
func (state *FanOutState) checkAndUpdateStatus() {
	if !state.WaitingForAll || state.Status != FanOutStatusWaiting {
		return
	}

	allComplete := true
	anyFailed := false

	for _, child := range state.Children {
		switch child.Status {
		case ChildStatusPending, ChildStatusRunning:
			allComplete = false
		case ChildStatusFailed, ChildStatusTimedOut:
			anyFailed = true
		}
	}

	if allComplete {
		now := time.Now()
		state.EndTime = &now
		if anyFailed {
			state.Status = FanOutStatusFailed
		} else {
			state.Status = FanOutStatusCompleted
		}
	}
}

// persist marshals the state data while holding the read lock.
// Must be called with state.mu held for reading.
func (state *FanOutState) persist() ([]byte, error) {
	return json.MarshalIndent(state, "", "  ")
}

// persistState saves pre-marshaled fan-out state data to disk.
// This method does not acquire locks - the caller must ensure proper synchronization.
func (sm *FanOutStateManager) persistState(stateID string, data []byte) error {
	stateFile := filepath.Join(sm.stateDir, fmt.Sprintf("%s.json", stateID))

	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %v", err)
	}

	return nil
}

// loadStates loads all existing fan-out states from disk.
func (sm *FanOutStateManager) loadStates() error {
	if _, err := os.Stat(sm.stateDir); os.IsNotExist(err) {
		return nil // No state directory exists yet
	}

	entries, err := os.ReadDir(sm.stateDir)
	if err != nil {
		return fmt.Errorf("failed to read state directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			if err := sm.loadStateFile(entry.Name()); err != nil {
				// Log error but continue loading other states
				fmt.Printf("Warning: failed to load state file %s: %v\n", entry.Name(), err)
			}
		}
	}

	return nil
}

// loadStateFile loads a single state file from disk.
func (sm *FanOutStateManager) loadStateFile(filename string) error {
	stateFile := filepath.Join(sm.stateDir, filename)

	data, err := os.ReadFile(stateFile)
	if err != nil {
		return fmt.Errorf("failed to read state file: %v", err)
	}

	var state FanOutState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to unmarshal state: %v", err)
	}

	// Restore runtime fields
	state.stateManager = sm

	sm.states[state.ID] = &state
	return nil
}

// ListActiveFanOuts returns all active (non-complete) fan-out operations.
func (sm *FanOutStateManager) ListActiveFanOuts() []FanOutSummary {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var active []FanOutSummary
	for _, state := range sm.states {
		if !state.IsComplete() {
			active = append(active, state.GetSummary())
		}
	}

	return active
}

// CleanupCompletedStates removes completed fan-out states older than the specified duration.
func (sm *FanOutStateManager) CleanupCompletedStates(olderThan time.Duration) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	var toDelete []string

	for id, state := range sm.states {
		if state.IsComplete() && state.EndTime != nil && state.EndTime.Before(cutoff) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		stateFile := filepath.Join(sm.stateDir, fmt.Sprintf("%s.json", id))
		if err := os.Remove(stateFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove state file %s: %v", stateFile, err)
		}
		delete(sm.states, id)
	}

	return nil
}

// IsWorkflowTriggered checks if a workflow has already been triggered for idempotency.
func (fs *FanOutState) IsWorkflowTriggered(repository, workflow string) (bool, string) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", repository, workflow)
	runID, exists := fs.TriggeredWorkflows[key]
	return exists, runID
}

// MarkWorkflowTriggered records that a workflow has been triggered to prevent duplicate execution.
func (fs *FanOutState) MarkWorkflowTriggered(repository, workflow, runID string) error {
	fs.mu.Lock()
	key := fmt.Sprintf("%s/%s", repository, workflow)
	fs.TriggeredWorkflows[key] = runID
	data, err := fs.persist()
	fs.mu.Unlock()

	if err != nil {
		return err
	}

	// Persist the state to ensure idempotency survives restarts
	if fs.stateManager != nil {
		return fs.stateManager.persistState(fs.ID, data)
	}

	return nil
}
