package engine

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FanOutState represents the state of a fan-out operation and its child workflows.
type FanOutState struct {
	ID            string                    `json:"id"`
	ParentRunID   string                    `json:"parent_run_id,omitempty"`
	SourceRepo    string                    `json:"source_repo"`
	EventType     string                    `json:"event_type"`
	Status        FanOutStatus              `json:"status"`
	StartTime     time.Time                 `json:"start_time"`
	EndTime       *time.Time                `json:"end_time,omitempty"`
	Children      map[string]*ChildWorkflow `json:"children"`
	WaitingForAll bool                      `json:"waiting_for_all"`
	Timeout       time.Duration             `json:"timeout,omitempty"`
	ErrorMessage  string                    `json:"error_message,omitempty"`

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
	stateDir             string
	mu                   sync.RWMutex
	states               map[string]*FanOutState
	idempotencyRetention time.Duration
}

// NewFanOutStateManager creates a new state manager for fan-out operations.
func NewFanOutStateManager(stateDir string) (*FanOutStateManager, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %v", err)
	}

	manager := &FanOutStateManager{
		stateDir:             stateDir,
		states:               make(map[string]*FanOutState),
		idempotencyRetention: 24 * time.Hour, // Default 24 hours for idempotent states
	}

	// Load existing states from disk
	if err := manager.loadStates(); err != nil {
		return nil, fmt.Errorf("failed to load existing states: %v", err)
	}

	return manager, nil
}

// CreateFanOutState creates a new fan-out state and persists it.
func (sm *FanOutStateManager) CreateFanOutState(id, parentRunID, sourceRepo, eventType string, waitingForAll bool, timeout time.Duration) (*FanOutState, error) {
	return sm.CreateFanOutStateWithFingerprint(id, "", parentRunID, sourceRepo, eventType, waitingForAll, timeout)
}

// CreateFanOutStateWithFingerprint creates a new fan-out state with optional fingerprint for idempotency.
// If fingerprint is provided, it uses fingerprint-based naming and atomic creation.
// If fingerprint is empty, it uses traditional timestamp-based naming.
func (sm *FanOutStateManager) CreateFanOutStateWithFingerprint(id, fingerprint, parentRunID, sourceRepo, eventType string, waitingForAll bool, timeout time.Duration) (*FanOutState, error) {
	if fingerprint != "" {
		// Use fingerprint-based ID and atomic creation
		fingerprintID := fmt.Sprintf("fanout-%s", fingerprint)
		return sm.createStateAtomic(fingerprintID, parentRunID, sourceRepo, eventType, waitingForAll, timeout)
	}

	// Traditional creation without fingerprint
	sm.mu.Lock()
	defer sm.mu.Unlock()

	state := &FanOutState{
		ID:            id,
		ParentRunID:   parentRunID,
		SourceRepo:    sourceRepo,
		EventType:     eventType,
		Status:        FanOutStatusPending,
		StartTime:     time.Now(),
		Children:      make(map[string]*ChildWorkflow),
		WaitingForAll: waitingForAll,
		Timeout:       timeout,
		stateManager:  sm,
	}

	sm.states[id] = state

	if err := sm.persistState(state); err != nil {
		delete(sm.states, id)
		return nil, fmt.Errorf("failed to persist state: %v", err)
	}

	return state, nil
}

// SetIdempotencyRetention sets the retention period for idempotent states.
// This only affects cleanup of states with fingerprint-based names.
func (sm *FanOutStateManager) SetIdempotencyRetention(retention time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.idempotencyRetention = retention
}

// GetIdempotencyRetention returns the current retention period for idempotent states.
func (sm *FanOutStateManager) GetIdempotencyRetention() time.Duration {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.idempotencyRetention
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

// GetFanOutStateByFingerprint retrieves a fan-out state by event fingerprint.
// Returns nil (not an error) if no state exists for the given fingerprint.
func (sm *FanOutStateManager) GetFanOutStateByFingerprint(fingerprint string) (*FanOutState, error) {
	fingerprintID := fmt.Sprintf("fanout-%s", fingerprint)

	sm.mu.RLock()
	state, exists := sm.states[fingerprintID]
	sm.mu.RUnlock()

	if !exists {
		// Check if the state file exists on disk but wasn't loaded
		stateFile := filepath.Join(sm.stateDir, fmt.Sprintf("%s.json", fingerprintID))
		if _, err := os.Stat(stateFile); err == nil {
			// File exists, try to load it
			if err := sm.loadStateFile(fmt.Sprintf("%s.json", fingerprintID)); err != nil {
				return nil, fmt.Errorf("failed to load state file for fingerprint %s: %v", fingerprint, err)
			}
			// Try again after loading
			sm.mu.RLock()
			state, exists = sm.states[fingerprintID]
			sm.mu.RUnlock()
		}
	}

	if !exists {
		return nil, nil // Not found, but not an error
	}

	return state, nil
}

// AddChildWorkflow adds a child workflow to the fan-out state.
func (state *FanOutState) AddChildWorkflow(repository, workflow string, inputs map[string]string) *ChildWorkflow {
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
	state.mu.Unlock()

	// Persist state after releasing lock
	state.stateManager.persistState(state)

	return child
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
	state.mu.Unlock()

	// Persist state after releasing lock
	return state.stateManager.persistState(state)
}

// StartFanOut marks the fan-out as running.
func (state *FanOutState) StartFanOut() error {
	state.mu.Lock()
	state.Status = FanOutStatusRunning
	state.mu.Unlock()

	return state.stateManager.persistState(state)
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
	state.mu.Unlock()

	return state.stateManager.persistState(state)
}

// CompleteFanOut marks the fan-out as completed.
func (state *FanOutState) CompleteFanOut() error {
	state.mu.Lock()
	state.Status = FanOutStatusCompleted
	now := time.Now()
	state.EndTime = &now
	state.mu.Unlock()

	return state.stateManager.persistState(state)
}

// FailFanOut marks the fan-out as failed.
func (state *FanOutState) FailFanOut(errorMessage string) error {
	state.mu.Lock()
	state.Status = FanOutStatusFailed
	state.ErrorMessage = errorMessage
	now := time.Now()
	state.EndTime = &now
	state.mu.Unlock()

	return state.stateManager.persistState(state)
}

// TimeoutFanOut marks the fan-out as timed out.
func (state *FanOutState) TimeoutFanOut() error {
	state.mu.Lock()
	state.Status = FanOutStatusTimedOut
	now := time.Now()
	state.EndTime = &now
	state.mu.Unlock()

	return state.stateManager.persistState(state)
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

// persistState saves the fan-out state to disk.
// The state mutex should be held for reading by the caller.
func (sm *FanOutStateManager) persistState(state *FanOutState) error {
	stateFile := filepath.Join(sm.stateDir, fmt.Sprintf("%s.json", state.ID))

	// Read state data under lock, then release before I/O
	state.mu.RLock()
	data, err := json.MarshalIndent(state, "", "  ")
	state.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("failed to marshal state: %v", err)
	}

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
// For idempotent states (those with fingerprint-based names), it uses the configured
// idempotency retention period instead of the provided duration.
func (sm *FanOutStateManager) CleanupCompletedStates(olderThan time.Duration) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	var toDelete []string

	for id, state := range sm.states {
		if !state.IsComplete() || state.EndTime == nil {
			continue
		}

		// Determine retention period based on state type
		var retentionPeriod time.Duration
		if sm.isIdempotentState(id) {
			// Use idempotency retention for fingerprint-based states
			retentionPeriod = sm.idempotencyRetention
		} else {
			// Use provided duration for traditional timestamp-based states
			retentionPeriod = olderThan
		}

		cutoff := now.Add(-retentionPeriod)
		if state.EndTime.Before(cutoff) {
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

// isIdempotentState checks if a state ID represents an idempotent state
// by checking if it follows the fingerprint-based naming pattern.
func (sm *FanOutStateManager) isIdempotentState(stateID string) bool {
	// Idempotent states have the pattern: "fanout-<fingerprint>"
	// where fingerprint is a hex string (SHA256 = 64 chars)
	if !strings.HasPrefix(stateID, "fanout-") {
		return false
	}

	suffix := strings.TrimPrefix(stateID, "fanout-")

	// Check if suffix looks like a hex fingerprint (64 chars, all hex)
	if len(suffix) == 64 {
		for _, char := range suffix {
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				return false
			}
		}
		return true
	}

	return false
}

// createStateAtomic creates a fan-out state using atomic file operations to handle race conditions.
// If a state with the same ID already exists, it loads and returns the existing state.
// Returns the state and a boolean indicating whether it was newly created (true) or existing (false).
func (sm *FanOutStateManager) createStateAtomic(id, parentRunID, sourceRepo, eventType string, waitingForAll bool, timeout time.Duration) (*FanOutState, error) {
	// Check if state already exists in memory
	sm.mu.RLock()
	if existingState, exists := sm.states[id]; exists {
		sm.mu.RUnlock()
		return existingState, nil
	}
	sm.mu.RUnlock()

	// Create new state
	state := &FanOutState{
		ID:            id,
		ParentRunID:   parentRunID,
		SourceRepo:    sourceRepo,
		EventType:     eventType,
		Status:        FanOutStatusPending,
		StartTime:     time.Now(),
		Children:      make(map[string]*ChildWorkflow),
		WaitingForAll: waitingForAll,
		Timeout:       timeout,
		stateManager:  sm,
	}

	// Generate temporary filename with random UUID
	tempID := make([]byte, 16)
	if _, err := rand.Read(tempID); err != nil {
		return nil, fmt.Errorf("failed to generate temp ID: %v", err)
	}
	tempFileName := fmt.Sprintf("%s.tmp.%x", id, tempID)
	tempFile := filepath.Join(sm.stateDir, fmt.Sprintf("%s.json", tempFileName))
	finalFile := filepath.Join(sm.stateDir, fmt.Sprintf("%s.json", id))

	// Marshal state data
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %v", err)
	}

	// Write to temporary file
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write temp state file: %v", err)
	}

	// Attempt atomic rename
	if err := os.Rename(tempFile, finalFile); err != nil {
		// Clean up temp file
		os.Remove(tempFile)

		// Check if the rename failed because the target already exists
		if os.IsExist(err) || (finalFile != "" && fileExists(finalFile)) {
			// Another process won the race, load the existing state
			if err := sm.loadStateFile(fmt.Sprintf("%s.json", id)); err != nil {
				return nil, fmt.Errorf("failed to load existing state after race condition: %v", err)
			}

			sm.mu.RLock()
			existingState, exists := sm.states[id]
			sm.mu.RUnlock()

			if !exists {
				return nil, fmt.Errorf("state should exist after loading but not found: %s", id)
			}

			return existingState, nil
		}

		return nil, fmt.Errorf("failed to rename temp file to final state file: %v", err)
	}

	// Successfully created new state, add to memory
	sm.mu.Lock()
	sm.states[id] = state
	sm.mu.Unlock()

	return state, nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GenerateEventFingerprint generates a deterministic fingerprint for an event to enable idempotency.
// It uses the event ID if available, otherwise falls back to a hash of the event properties.
func GenerateEventFingerprint(event interface{}) (string, error) {
	switch e := event.(type) {
	case *EnhancedEvent:
		// Use event ID if available
		if e.Metadata.ID != "" {
			return e.Metadata.ID, nil
		}
		// Fallback to hash
		return generateEventHash(e.Type, e.Metadata.Source, e.Payload)
	case *Event:
		// Legacy event - always use hash
		return generateEventHash(e.Type, e.Source, e.Payload)
	default:
		return "", fmt.Errorf("unsupported event type: %T", event)
	}
}

// generateEventHash creates a SHA256 hash from event properties.
func generateEventHash(eventType, source string, payload map[string]interface{}) (string, error) {
	// Normalize the payload for consistent hashing
	normalizedPayload, err := normalizePayload(payload)
	if err != nil {
		return "", fmt.Errorf("failed to normalize payload: %v", err)
	}

	// Create a composite key from event properties
	composite := map[string]interface{}{
		"type":    eventType,
		"source":  source,
		"payload": normalizedPayload,
	}

	// Convert to canonical JSON
	data, err := json.Marshal(composite)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event for hashing: %v", err)
	}

	// Generate SHA256 hash
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// normalizePayload creates a normalized representation of a payload for consistent hashing.
// It handles nested structures and ensures deterministic ordering.
func normalizePayload(payload map[string]interface{}) (map[string]interface{}, error) {
	if payload == nil {
		return nil, nil
	}

	normalized := make(map[string]interface{})

	// Get sorted keys for deterministic ordering
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Process each key in sorted order
	for _, key := range keys {
		value := payload[key]
		normalizedValue, err := normalizeValue(value)
		if err != nil {
			return nil, fmt.Errorf("failed to normalize key %s: %v", key, err)
		}
		normalized[key] = normalizedValue
	}

	return normalized, nil
}

// normalizeValue recursively normalizes a value for consistent representation.
func normalizeValue(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case map[string]interface{}:
		// Recursively normalize nested maps
		return normalizePayload(v)
	case []interface{}:
		// Normalize each element in the slice
		normalized := make([]interface{}, len(v))
		for i, elem := range v {
			normalizedElem, err := normalizeValue(elem)
			if err != nil {
				return nil, err
			}
			normalized[i] = normalizedElem
		}
		return normalized, nil
	case float64, int, int64, string, bool, nil:
		// Primitive types are already normalized
		return v, nil
	default:
		// Convert other numeric types to float64 for consistency
		// This handles cases where JSON unmarshaling might produce different numeric types
		switch v := v.(type) {
		case int32:
			return float64(v), nil
		case int16:
			return float64(v), nil
		case int8:
			return float64(v), nil
		case uint:
			return float64(v), nil
		case uint64:
			return float64(v), nil
		case uint32:
			return float64(v), nil
		case uint16:
			return float64(v), nil
		case uint8:
			return float64(v), nil
		case float32:
			return float64(v), nil
		default:
			// For unknown types, convert to string representation
			return fmt.Sprintf("%v", v), nil
		}
	}
}
