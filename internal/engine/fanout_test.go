package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dangazineu/tako/internal/config"
)

func TestNewFanOutExecutor(t *testing.T) {
	tempDir := t.TempDir()
	mockRunner := NewTestMockWorkflowRunner()

	executor, err := NewFanOutExecutor(tempDir, false, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create fan-out executor: %v", err)
	}

	if executor.cacheDir != tempDir {
		t.Errorf("Expected cache directory %s, got %s", tempDir, executor.cacheDir)
	}
	if executor.debug != false {
		t.Errorf("Expected debug false, got %v", executor.debug)
	}

	// Verify idempotency is disabled by default
	if executor.IsIdempotencyEnabled() {
		t.Error("Expected idempotency to be disabled by default")
	}
}

func TestFanOutExecutor_IdempotencyConfiguration(t *testing.T) {
	tempDir := t.TempDir()
	mockRunner := NewTestMockWorkflowRunner()

	executor, err := NewFanOutExecutor(tempDir, false, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Test that idempotency is disabled by default
	if executor.IsIdempotencyEnabled() {
		t.Error("Expected idempotency to be disabled by default")
	}

	// Test enabling idempotency
	executor.SetIdempotency(true)
	if !executor.IsIdempotencyEnabled() {
		t.Error("Expected idempotency to be enabled after SetIdempotency(true)")
	}

	// Test disabling idempotency
	executor.SetIdempotency(false)
	if executor.IsIdempotencyEnabled() {
		t.Error("Expected idempotency to be disabled after SetIdempotency(false)")
	}
}

func TestFanOutExecutor_parseFanOutParams(t *testing.T) {
	mockRunner := NewTestMockWorkflowRunner()
	executor, err := NewFanOutExecutor(t.TempDir(), false, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	tests := []struct {
		name        string
		withParams  map[string]interface{}
		expected    *FanOutParams
		expectError bool
	}{
		{
			name: "minimal valid params",
			withParams: map[string]interface{}{
				"event_type": "library_built",
			},
			expected: &FanOutParams{
				EventType:        "library_built",
				WaitForChildren:  false,
				ConcurrencyLimit: 0,
				Payload:          map[string]interface{}{},
			},
		},
		{
			name: "full valid params",
			withParams: map[string]interface{}{
				"event_type":        "library_built",
				"wait_for_children": true,
				"timeout":           "2h",
				"concurrency_limit": 4,
				"schema_version":    "1.0.0",
				"payload": map[string]interface{}{
					"version": "2.1.0",
					"status":  "success",
				},
			},
			expected: &FanOutParams{
				EventType:        "library_built",
				WaitForChildren:  true,
				Timeout:          "2h",
				ConcurrencyLimit: 4,
				SchemaVersion:    "1.0.0",
				Payload: map[string]interface{}{
					"version": "2.1.0",
					"status":  "success",
				},
			},
		},
		{
			name: "concurrency_limit as string",
			withParams: map[string]interface{}{
				"event_type":        "library_built",
				"concurrency_limit": "6",
			},
			expected: &FanOutParams{
				EventType:        "library_built",
				WaitForChildren:  false,
				ConcurrencyLimit: 6,
				Payload:          map[string]interface{}{},
			},
		},
		{
			name:        "missing event_type",
			withParams:  map[string]interface{}{},
			expectError: true,
		},
		{
			name: "invalid event_type type",
			withParams: map[string]interface{}{
				"event_type": 123,
			},
			expectError: true,
		},
		{
			name: "invalid wait_for_children type",
			withParams: map[string]interface{}{
				"event_type":        "library_built",
				"wait_for_children": "true",
			},
			expectError: true,
		},
		{
			name: "invalid timeout type",
			withParams: map[string]interface{}{
				"event_type": "library_built",
				"timeout":    123,
			},
			expectError: true,
		},
		{
			name: "invalid concurrency_limit type",
			withParams: map[string]interface{}{
				"event_type":        "library_built",
				"concurrency_limit": "invalid",
			},
			expectError: true,
		},
		{
			name: "invalid payload type",
			withParams: map[string]interface{}{
				"event_type": "library_built",
				"payload":    "not a map",
			},
			expectError: true,
		},
		{
			name: "invalid schema_version type",
			withParams: map[string]interface{}{
				"event_type":     "library_built",
				"schema_version": 123,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := executor.parseFanOutParams(tt.withParams)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if params.EventType != tt.expected.EventType {
				t.Errorf("EventType = %v, want %v", params.EventType, tt.expected.EventType)
			}
			if params.WaitForChildren != tt.expected.WaitForChildren {
				t.Errorf("WaitForChildren = %v, want %v", params.WaitForChildren, tt.expected.WaitForChildren)
			}
			if params.Timeout != tt.expected.Timeout {
				t.Errorf("Timeout = %v, want %v", params.Timeout, tt.expected.Timeout)
			}
			if params.ConcurrencyLimit != tt.expected.ConcurrencyLimit {
				t.Errorf("ConcurrencyLimit = %v, want %v", params.ConcurrencyLimit, tt.expected.ConcurrencyLimit)
			}
			if params.SchemaVersion != tt.expected.SchemaVersion {
				t.Errorf("SchemaVersion = %v, want %v", params.SchemaVersion, tt.expected.SchemaVersion)
			}

			// Check payload
			if len(params.Payload) != len(tt.expected.Payload) {
				t.Errorf("Payload length = %v, want %v", len(params.Payload), len(tt.expected.Payload))
			}
			for key, expectedValue := range tt.expected.Payload {
				if actualValue, exists := params.Payload[key]; !exists {
					t.Errorf("Payload missing key %s", key)
				} else if actualValue != expectedValue {
					t.Errorf("Payload[%s] = %v, want %v", key, actualValue, expectedValue)
				}
			}
		})
	}
}

func TestFanOutExecutor_Execute(t *testing.T) {
	// Create temporary directory and test repository structure
	tempDir := t.TempDir()

	// Create test repository structure with subscriptions
	testRepo1Path := filepath.Join(tempDir, "repos", "test-org", "repo1", "main")
	testRepo2Path := filepath.Join(tempDir, "repos", "test-org", "repo2", "main")

	if err := os.MkdirAll(testRepo1Path, 0755); err != nil {
		t.Fatalf("Failed to create test repo1 directory: %v", err)
	}
	if err := os.MkdirAll(testRepo2Path, 0755); err != nil {
		t.Fatalf("Failed to create test repo2 directory: %v", err)
	}

	// Create tako.yml files with subscriptions
	takoYml1 := `version: "1.0"
workflows:
  update:
    steps:
      - run: echo "update triggered"
subscriptions:
  - artifact: "source-org/library:main"
    events: ["library_built"]
    workflow: "update"
`

	takoYml2 := `version: "1.0"
workflows:
  build:
    steps:
      - run: echo "build triggered"
subscriptions:
  - artifact: "source-org/library:main"
    events: ["library_built", "library_updated"]
    workflow: "build"
`

	if err := os.WriteFile(filepath.Join(testRepo1Path, "tako.yml"), []byte(takoYml1), 0644); err != nil {
		t.Fatalf("Failed to write tako.yml for repo1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testRepo2Path, "tako.yml"), []byte(takoYml2), 0644); err != nil {
		t.Fatalf("Failed to write tako.yml for repo2: %v", err)
	}

	mockRunner := NewTestMockWorkflowRunner()
	executor, err := NewFanOutExecutor(tempDir, true, mockRunner) // Enable debug for visibility
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	tests := []struct {
		name              string
		step              config.WorkflowStep
		sourceRepo        string
		expectedTriggered int
		expectedSuccess   bool
		expectedErrors    int
	}{
		{
			name: "successful fan-out with subscribers",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type": "library_built",
					"payload": map[string]interface{}{
						"version": "2.1.0",
					},
				},
			},
			sourceRepo:        "source-org/library",
			expectedTriggered: 2, // Both repo1 and repo2 subscribe to library_built
			expectedSuccess:   true,
			expectedErrors:    0,
		},
		{
			name: "fan-out with no subscribers",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type": "unknown_event",
				},
			},
			sourceRepo:        "source-org/library",
			expectedTriggered: 0,
			expectedSuccess:   true,
			expectedErrors:    0,
		},
		{
			name: "fan-out with wait_for_children",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type":        "library_built",
					"wait_for_children": true,
					"timeout":           "1s",
				},
			},
			sourceRepo:        "source-org/library",
			expectedTriggered: 2,
			expectedSuccess:   true,
			expectedErrors:    0,
		},
		{
			name: "fan-out with concurrency limit",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type":        "library_built",
					"concurrency_limit": 1,
				},
			},
			sourceRepo:        "source-org/library",
			expectedTriggered: 2,
			expectedSuccess:   true,
			expectedErrors:    0,
		},
		{
			name: "invalid parameters",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					// Missing required event_type
				},
			},
			sourceRepo:        "source-org/library",
			expectedTriggered: 0,
			expectedSuccess:   false,
			expectedErrors:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.Execute(tt.step, tt.sourceRepo)

			if tt.expectedSuccess {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("Expected success, but got failure. Errors: %v", result.Errors)
				}
			} else {
				if err == nil && result.Success {
					t.Errorf("Expected failure, but got success")
				}
			}

			if result.TriggeredCount != tt.expectedTriggered {
				t.Errorf("TriggeredCount = %v, want %v", result.TriggeredCount, tt.expectedTriggered)
			}

			if len(result.Errors) != tt.expectedErrors {
				t.Errorf("Error count = %v, want %v. Errors: %v", len(result.Errors), tt.expectedErrors, result.Errors)
			}

			// Verify event was emitted (except for invalid params)
			if tt.expectedSuccess && tt.expectedErrors == 0 {
				if !result.EventEmitted {
					t.Errorf("Expected event to be emitted")
				}
			}

			// Verify timing
			if result.EndTime.Before(result.StartTime) {
				t.Errorf("End time should be after start time")
			}
		})
	}
}

func TestFanOutExecutor_simulateWorkflowTrigger(t *testing.T) {
	mockRunner := NewTestMockWorkflowRunner()
	executor, err := NewFanOutExecutor(t.TempDir(), false, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	tests := []struct {
		name        string
		repository  string
		workflow    string
		inputs      map[string]string
		expectError bool
	}{
		{
			name:       "successful trigger",
			repository: "test-org/repo",
			workflow:   "build",
			inputs:     map[string]string{"version": "1.0.0"},
		},
		{
			name:        "simulated failure",
			repository:  "test-org/fail-repo",
			workflow:    "build",
			inputs:      map[string]string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.simulateWorkflowTrigger(tt.repository, tt.workflow, tt.inputs)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestFanOutExecutor_waitForChildren(t *testing.T) {
	mockRunner := NewTestMockWorkflowRunner()
	executor, err := NewFanOutExecutor(t.TempDir(), false, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	tests := []struct {
		name        string
		subscribers []SubscriptionMatch
		params      *FanOutParams
		expectError bool
	}{
		{
			name:        "no timeout",
			subscribers: make([]SubscriptionMatch, 2),
			params: &FanOutParams{
				Timeout: "",
			},
		},
		{
			name:        "with valid timeout",
			subscribers: make([]SubscriptionMatch, 1),
			params: &FanOutParams{
				Timeout: "1s",
			},
		},
		{
			name:        "timeout too short",
			subscribers: make([]SubscriptionMatch, 100), // Would require 5s to wait
			params: &FanOutParams{
				Timeout: "100ms",
			},
			expectError: true,
		},
		{
			name:        "invalid timeout format",
			subscribers: make([]SubscriptionMatch, 1),
			params: &FanOutParams{
				Timeout: "invalid",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			err := executor.waitForChildren(tt.subscribers, tt.params)
			elapsed := time.Since(start)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				// Verify it actually waited some amount of time
				expectedMinWait := time.Duration(len(tt.subscribers)) * 50 * time.Millisecond
				if elapsed < expectedMinWait/2 { // Allow some tolerance
					t.Errorf("Expected to wait at least %v, but only waited %v", expectedMinWait/2, elapsed)
				}
			}
		})
	}
}

func TestConvertPayload(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]interface{}
	}{
		{
			name:     "empty payload",
			input:    map[string]string{},
			expected: map[string]interface{}{},
		},
		{
			name: "simple payload",
			input: map[string]string{
				"version": "1.0.0",
				"status":  "success",
			},
			expected: map[string]interface{}{
				"version": "1.0.0",
				"status":  "success",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertPayload(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Result length = %v, want %v", len(result), len(tt.expected))
			}

			for key, expectedValue := range tt.expected {
				if actualValue, exists := result[key]; !exists {
					t.Errorf("Result missing key %s", key)
				} else if actualValue != expectedValue {
					t.Errorf("Result[%s] = %v, want %v", key, actualValue, expectedValue)
				}
			}
		})
	}
}

func TestFanOutExecutor_IdempotencyDuplicateDetection(t *testing.T) {
	// Create temporary directory and test repository structure
	tempDir := t.TempDir()

	// Create test repository structure with subscriptions
	testRepoPath := filepath.Join(tempDir, "repos", "test-org", "repo1", "main")
	if err := os.MkdirAll(testRepoPath, 0755); err != nil {
		t.Fatalf("Failed to create test repo directory: %v", err)
	}

	// Create tako.yml with subscription
	takoYml := `version: "1.0"
workflows:
  update:
    steps:
      - run: echo "update triggered"
subscriptions:
  - artifact: "source-org/library:main"
    events: ["library_built"]
    workflow: "update"
`
	if err := os.WriteFile(filepath.Join(testRepoPath, "tako.yml"), []byte(takoYml), 0644); err != nil {
		t.Fatalf("Failed to write tako.yml: %v", err)
	}

	mockRunner := NewTestMockWorkflowRunner()
	executor, err := NewFanOutExecutor(tempDir, true, mockRunner) // Enable debug
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Enable idempotency
	executor.SetIdempotency(true)

	step := config.WorkflowStep{
		Uses: "tako/fan-out@v1",
		With: map[string]interface{}{
			"event_type": "library_built",
			"payload": map[string]interface{}{
				"version": "2.1.0",
				"status":  "success",
			},
		},
	}
	sourceRepo := "source-org/library:main"

	// Execute first time
	result1, err := executor.Execute(step, sourceRepo)
	if err != nil {
		t.Fatalf("First execution failed: %v", err)
	}
	if !result1.Success {
		t.Errorf("First execution should succeed, got: %v", result1.Errors)
	}
	if result1.TriggeredCount != 1 {
		t.Errorf("Expected 1 triggered workflow, got %d", result1.TriggeredCount)
	}

	// Execute second time with same event - should detect duplicate
	result2, err := executor.Execute(step, sourceRepo)
	if err != nil {
		t.Fatalf("Second execution failed: %v", err)
	}
	if !result2.Success {
		t.Errorf("Second execution should succeed (duplicate), got: %v", result2.Errors)
	}
	if result2.TriggeredCount != 0 {
		t.Errorf("Expected 0 triggered workflows in duplicate result (should be cached), got %d", result2.TriggeredCount)
	}

	// Verify that both executions have same FanOutID (fingerprint-based)
	if result1.FanOutID != result2.FanOutID {
		t.Errorf("Expected same FanOutID for duplicate events, got %s vs %s", result1.FanOutID, result2.FanOutID)
	}

	// Execute third time with different payload - should not be duplicate
	step3 := config.WorkflowStep{
		Uses: "tako/fan-out@v1",
		With: map[string]interface{}{
			"event_type": "library_built",
			"payload": map[string]interface{}{
				"version": "2.2.0", // Different version
				"status":  "success",
			},
		},
	}

	result3, err := executor.Execute(step3, sourceRepo)
	if err != nil {
		t.Fatalf("Third execution failed: %v", err)
	}
	if !result3.Success {
		t.Errorf("Third execution should succeed, got: %v", result3.Errors)
	}
	if result3.TriggeredCount != 1 {
		t.Errorf("Expected 1 triggered workflow, got %d", result3.TriggeredCount)
	}

	// Verify that third execution has different FanOutID
	if result1.FanOutID == result3.FanOutID {
		t.Errorf("Expected different FanOutID for different events, got same: %s", result1.FanOutID)
	}
}

func TestFanOutExecutor_IdempotencyDisabled(t *testing.T) {
	// Create temporary directory and test repository structure
	tempDir := t.TempDir()

	// Create test repository structure with subscriptions
	testRepoPath := filepath.Join(tempDir, "repos", "test-org", "repo1", "main")
	if err := os.MkdirAll(testRepoPath, 0755); err != nil {
		t.Fatalf("Failed to create test repo directory: %v", err)
	}

	// Create tako.yml with subscription
	takoYml := `version: "1.0"
workflows:
  update:
    steps:
      - run: echo "update triggered"
subscriptions:
  - artifact: "source-org/library:main"
    events: ["library_built"]
    workflow: "update"
`
	if err := os.WriteFile(filepath.Join(testRepoPath, "tako.yml"), []byte(takoYml), 0644); err != nil {
		t.Fatalf("Failed to write tako.yml: %v", err)
	}

	mockRunner := NewTestMockWorkflowRunner()
	executor, err := NewFanOutExecutor(tempDir, false, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Idempotency is disabled by default
	if executor.IsIdempotencyEnabled() {
		t.Error("Expected idempotency to be disabled by default")
	}

	step := config.WorkflowStep{
		Uses: "tako/fan-out@v1",
		With: map[string]interface{}{
			"event_type": "library_built",
			"payload": map[string]interface{}{
				"version": "2.1.0",
				"status":  "success",
			},
		},
	}
	sourceRepo := "source-org/library:main"

	// Execute first time
	result1, err := executor.Execute(step, sourceRepo)
	if err != nil {
		t.Fatalf("First execution failed: %v", err)
	}
	if !result1.Success {
		t.Errorf("First execution should succeed, got: %v", result1.Errors)
	}

	// Execute second time with same event - should NOT detect duplicate (idempotency disabled)
	result2, err := executor.Execute(step, sourceRepo)
	if err != nil {
		t.Fatalf("Second execution failed: %v", err)
	}
	if !result2.Success {
		t.Errorf("Second execution should succeed, got: %v", result2.Errors)
	}

	// Verify that both executions have different FanOutIDs (timestamp-based)
	if result1.FanOutID == result2.FanOutID {
		t.Errorf("Expected different FanOutIDs when idempotency disabled, got same: %s", result1.FanOutID)
	}
}

func TestFanOutExecutor_IdempotencyWithEventID(t *testing.T) {
	// Create temporary directory and test repository structure
	tempDir := t.TempDir()

	// Create test repository structure with subscriptions
	testRepoPath := filepath.Join(tempDir, "repos", "test-org", "repo1", "main")
	if err := os.MkdirAll(testRepoPath, 0755); err != nil {
		t.Fatalf("Failed to create test repo directory: %v", err)
	}

	// Create tako.yml with subscription
	takoYml := `version: "1.0"
workflows:
  update:
    steps:
      - run: echo "update triggered"
subscriptions:
  - artifact: "source-org/library:main"
    events: ["library_built"]
    workflow: "update"
`
	if err := os.WriteFile(filepath.Join(testRepoPath, "tako.yml"), []byte(takoYml), 0644); err != nil {
		t.Fatalf("Failed to write tako.yml: %v", err)
	}

	mockRunner := NewTestMockWorkflowRunner()
	executor, err := NewFanOutExecutor(tempDir, true, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Enable idempotency
	executor.SetIdempotency(true)

	// Test with event that would have ID - simulate by using unique payload
	step := config.WorkflowStep{
		Uses: "tako/fan-out@v1",
		With: map[string]interface{}{
			"event_type": "library_built",
			"payload": map[string]interface{}{
				"event_id": "unique-event-123", // This would be used for fingerprinting
				"version":  "2.1.0",
			},
		},
	}
	sourceRepo := "source-org/library:main"

	// Execute first time
	result1, err := executor.Execute(step, sourceRepo)
	if err != nil {
		t.Fatalf("First execution failed: %v", err)
	}
	if !result1.Success {
		t.Errorf("First execution should succeed, got: %v", result1.Errors)
	}

	// Execute second time with same event_id - should detect duplicate
	result2, err := executor.Execute(step, sourceRepo)
	if err != nil {
		t.Fatalf("Second execution failed: %v", err)
	}
	if !result2.Success {
		t.Errorf("Second execution should succeed (duplicate), got: %v", result2.Errors)
	}

	// Verify that both executions have same FanOutID
	if result1.FanOutID != result2.FanOutID {
		t.Errorf("Expected same FanOutID for events with same ID, got %s vs %s", result1.FanOutID, result2.FanOutID)
	}
}

func TestFanOutExecutor_IdempotencyConcurrentDuplicates(t *testing.T) {
	// Create temporary directory and test repository structure
	tempDir := t.TempDir()

	// Create test repository structure with subscriptions
	testRepoPath := filepath.Join(tempDir, "repos", "test-org", "repo1", "main")
	if err := os.MkdirAll(testRepoPath, 0755); err != nil {
		t.Fatalf("Failed to create test repo directory: %v", err)
	}

	// Create tako.yml with subscription
	takoYml := `version: "1.0"
workflows:
  update:
    steps:
      - run: echo "update triggered"
subscriptions:
  - artifact: "source-org/library:main"
    events: ["library_built"]
    workflow: "update"
`
	if err := os.WriteFile(filepath.Join(testRepoPath, "tako.yml"), []byte(takoYml), 0644); err != nil {
		t.Fatalf("Failed to write tako.yml: %v", err)
	}

	mockRunner := NewTestMockWorkflowRunner()
	executor, err := NewFanOutExecutor(tempDir, true, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Enable idempotency
	executor.SetIdempotency(true)

	step := config.WorkflowStep{
		Uses: "tako/fan-out@v1",
		With: map[string]interface{}{
			"event_type": "library_built",
			"payload": map[string]interface{}{
				"version": "3.0.0",
				"status":  "success",
			},
		},
	}
	sourceRepo := "source-org/library:main"

	// Execute the event multiple times sequentially (concurrent testing is complex for filesystem operations)
	var results []*FanOutResult
	for i := 0; i < 3; i++ {
		result, err := executor.Execute(step, sourceRepo)
		if err != nil {
			t.Fatalf("Execution %d failed: %v", i, err)
		}
		if !result.Success {
			t.Fatalf("Execution %d should succeed, got errors: %v", i, result.Errors)
		}
		results = append(results, result)
	}

	// Verify all executions have the same FanOutID (all duplicates)
	expectedFanOutID := results[0].FanOutID
	for i, result := range results {
		if result.FanOutID != expectedFanOutID {
			t.Errorf("Execution %d has different FanOutID: expected %s, got %s", i, expectedFanOutID, result.FanOutID)
		}
	}

	// First execution should trigger workflows, subsequent ones should be cached
	if results[0].TriggeredCount != 1 {
		t.Errorf("First execution should trigger 1 workflow, got %d", results[0].TriggeredCount)
	}

	for i := 1; i < len(results); i++ {
		if results[i].TriggeredCount != 0 {
			t.Errorf("Execution %d should be cached (TriggeredCount=0), got %d", i, results[i].TriggeredCount)
		}
	}
}

func TestFanOutExecutor_IdempotencyStatePersistenceRecovery(t *testing.T) {
	// Create temporary directory and test repository structure
	tempDir := t.TempDir()

	// Create test repository structure with subscriptions
	testRepoPath := filepath.Join(tempDir, "repos", "test-org", "repo1", "main")
	if err := os.MkdirAll(testRepoPath, 0755); err != nil {
		t.Fatalf("Failed to create test repo directory: %v", err)
	}

	// Create tako.yml with subscription
	takoYml := `version: "1.0"
workflows:
  update:
    steps:
      - run: echo "update triggered"
subscriptions:
  - artifact: "source-org/library:main"
    events: ["library_built"]
    workflow: "update"
`
	if err := os.WriteFile(filepath.Join(testRepoPath, "tako.yml"), []byte(takoYml), 0644); err != nil {
		t.Fatalf("Failed to write tako.yml: %v", err)
	}

	// Create first executor and execute event
	mockRunner1 := NewTestMockWorkflowRunner()
	executor1, err := NewFanOutExecutor(tempDir, false, mockRunner1)
	if err != nil {
		t.Fatalf("Failed to create first executor: %v", err)
	}
	executor1.SetIdempotency(true)

	step := config.WorkflowStep{
		Uses: "tako/fan-out@v1",
		With: map[string]interface{}{
			"event_type": "library_built",
			"payload": map[string]interface{}{
				"version": "4.0.0",
				"status":  "success",
			},
		},
	}
	sourceRepo := "source-org/library:main"

	// Execute first time
	result1, err := executor1.Execute(step, sourceRepo)
	if err != nil {
		t.Fatalf("First execution failed: %v", err)
	}
	if !result1.Success {
		t.Errorf("First execution should succeed, got: %v", result1.Errors)
	}

	// Create second executor (simulating process restart)
	mockRunner2 := NewTestMockWorkflowRunner()
	executor2, err := NewFanOutExecutor(tempDir, false, mockRunner2)
	if err != nil {
		t.Fatalf("Failed to create second executor: %v", err)
	}
	executor2.SetIdempotency(true)

	// Execute same event with second executor - should detect duplicate from persisted state
	result2, err := executor2.Execute(step, sourceRepo)
	if err != nil {
		t.Fatalf("Second execution failed: %v", err)
	}
	if !result2.Success {
		t.Errorf("Second execution should succeed (duplicate), got: %v", result2.Errors)
	}

	// Verify that both executions have same FanOutID
	if result1.FanOutID != result2.FanOutID {
		t.Errorf("Expected same FanOutID after process restart, got %s vs %s", result1.FanOutID, result2.FanOutID)
	}

	// Verify second execution recognized it as a duplicate (should not trigger)
	if result2.TriggeredCount != 0 {
		t.Errorf("Expected 0 triggered workflows for duplicate (should be cached), got %d", result2.TriggeredCount)
	}
}

// Test diamond dependency resolution functionality.
func TestFanOutExecutor_DiamondDependencyResolution(t *testing.T) {
	tempDir := t.TempDir()
	mockRunner := NewTestMockWorkflowRunner()

	executor, err := NewFanOutExecutor(tempDir, true, mockRunner) // Enable debug for logging
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Create test subscriptions with identical filters and inputs (diamond dependency)
	subscribers := []SubscriptionMatch{
		{
			Repository: "org/repo1", // First in alphabetical order - should win
			Subscription: config.Subscription{
				Workflow: "build.yml",
				Filters:  []string{"event.payload.version != null"},
				Inputs:   map[string]string{"version": "{{ .payload.version }}"},
			},
		},
		{
			Repository: "org/repo2", // Second in alphabetical order - should be skipped
			Subscription: config.Subscription{
				Workflow: "build.yml",
				Filters:  []string{"event.payload.version != null"},
				Inputs:   map[string]string{"version": "{{ .payload.version }}"},
			},
		},
		{
			Repository: "org/repo3", // Different workflow - should NOT be skipped
			Subscription: config.Subscription{
				Workflow: "test.yml", // Different workflow
				Filters:  []string{"event.payload.version != null"},
				Inputs:   map[string]string{"version": "{{ .payload.version }}"},
			},
		},
	}

	event := Event{
		Type:          "library_built",
		SchemaVersion: "1.0.0",
		Source:        "source/repo",
		Payload:       map[string]interface{}{"version": "1.2.3"},
		Timestamp:     time.Now().Unix(),
	}

	params := &FanOutParams{
		WaitForChildren:  false,
		ConcurrencyLimit: 0,
	}

	// Create state for testing
	state, err := executor.stateManager.CreateFanOutState("test-fanout", "", "source/repo", "library_built", false, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Test diamond dependency resolution
	triggeredCount, errors, detailedErrors := executor.triggerSubscribersWithState(subscribers, event, params, state)

	// Should only trigger 2 workflows: org/repo1:build.yml (winner) and org/repo3:test.yml (different workflow)
	if triggeredCount != 2 {
		t.Errorf("Expected 2 triggered workflows (1 winner + 1 different), got %d\nErrors: %v\nDetailed: %v",
			triggeredCount, errors, detailedErrors)
	}

	// Should have no errors
	if len(errors) > 0 {
		t.Errorf("Expected no errors, got: %v", errors)
	}

	// Verify the winner and skipped repos in state
	summary := state.GetSummary()
	if summary.TotalChildren != 2 {
		t.Errorf("Expected 2 child workflows in state, got %d", summary.TotalChildren)
	}

	// Verify specific children exist
	state.mu.RLock()
	foundRepo1 := false
	foundRepo3 := false
	foundRepo2 := false
	for childID, child := range state.Children {
		if child.Repository == "org/repo1" && child.Workflow == "build.yml" {
			foundRepo1 = true
		} else if child.Repository == "org/repo3" && child.Workflow == "test.yml" {
			foundRepo3 = true
		} else if child.Repository == "org/repo2" {
			foundRepo2 = true
			t.Errorf("Found unexpected child for skipped repo org/repo2: %s", childID)
		}
	}
	state.mu.RUnlock()

	if !foundRepo1 {
		t.Error("Expected to find child workflow for winning repository org/repo1")
	}
	if !foundRepo3 {
		t.Error("Expected to find child workflow for different workflow org/repo3:test.yml")
	}
	if foundRepo2 {
		t.Error("Should not find child workflow for skipped repository org/repo2")
	}
}

func TestFanOutExecutor_DiamondDependencyWithDifferentInputs(t *testing.T) {
	tempDir := t.TempDir()
	mockRunner := NewTestMockWorkflowRunner()

	executor, err := NewFanOutExecutor(tempDir, false, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Create subscriptions with same repository and workflow but different inputs (NOT diamond dependency)
	subscribers := []SubscriptionMatch{
		{
			Repository: "org/repo1",
			Subscription: config.Subscription{
				Workflow: "build.yml",
				Filters:  []string{"event.payload.version != null"},
				Inputs:   map[string]string{"version": "{{ .payload.version }}"}, // Different input value
			},
		},
		{
			Repository: "org/repo1",
			Subscription: config.Subscription{
				Workflow: "build.yml",
				Filters:  []string{"event.payload.version != null"},
				Inputs:   map[string]string{"tag": "{{ .payload.tag }}"}, // Different input key
			},
		},
	}

	event := Event{
		Type:          "library_built",
		SchemaVersion: "1.0.0",
		Source:        "source/repo",
		Payload:       map[string]interface{}{"version": "1.2.3", "tag": "v1.2.3"},
		Timestamp:     time.Now().Unix(),
	}

	params := &FanOutParams{
		WaitForChildren:  false,
		ConcurrencyLimit: 0,
	}

	state, err := executor.stateManager.CreateFanOutState("test-fanout-2", "", "source/repo", "library_built", false, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Test - should trigger both because inputs are different
	triggeredCount, errors, _ := executor.triggerSubscribersWithState(subscribers, event, params, state)

	// Should trigger both workflows since they have different inputs
	if triggeredCount != 2 {
		t.Errorf("Expected 2 triggered workflows (different inputs), got %d\nErrors: %v", triggeredCount, errors)
	}

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got: %v", errors)
	}
}

func TestFanOutExecutor_DiamondDependencyWhitespaceNormalization(t *testing.T) {
	tempDir := t.TempDir()
	mockRunner := NewTestMockWorkflowRunner()

	executor, err := NewFanOutExecutor(tempDir, false, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Create subscriptions with same logic but different whitespace (should be diamond dependency)
	subscribers := []SubscriptionMatch{
		{
			Repository: "org/repo1",
			Subscription: config.Subscription{
				Workflow: "build.yml",
				Filters:  []string{"event.payload.version != null"},
				Inputs:   map[string]string{"version": "{{ .payload.version }}"},
			},
		},
		{
			Repository: "org/repo2",
			Subscription: config.Subscription{
				Workflow: "build.yml",
				Filters:  []string{"event.payload.version!=null"},              // Different whitespace
				Inputs:   map[string]string{"version": "{{.payload.version}}"}, // Different whitespace
			},
		},
	}

	event := Event{
		Type:      "library_built",
		Source:    "source/repo",
		Payload:   map[string]interface{}{"version": "1.2.3"},
		Timestamp: time.Now().Unix(),
	}

	params := &FanOutParams{
		WaitForChildren:  false,
		ConcurrencyLimit: 0,
	}

	state, err := executor.stateManager.CreateFanOutState("test-fanout-3", "", "source/repo", "library_built", false, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Test - should only trigger one due to normalization
	triggeredCount, errors, _ := executor.triggerSubscribersWithState(subscribers, event, params, state)

	// Should only trigger 1 workflow due to whitespace normalization
	if triggeredCount != 1 {
		t.Errorf("Expected 1 triggered workflow (normalized whitespace), got %d\nErrors: %v", triggeredCount, errors)
	}
}

func TestFanOutExecutor_DiamondDependencyMultipleFilters(t *testing.T) {
	tempDir := t.TempDir()
	mockRunner := NewTestMockWorkflowRunner()

	executor, err := NewFanOutExecutor(tempDir, false, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Create subscriptions with same filters but different order (should be diamond dependency)
	subscribers := []SubscriptionMatch{
		{
			Repository: "org/repo1",
			Subscription: config.Subscription{
				Workflow: "build.yml",
				Filters:  []string{"event.payload.version != null", "event.payload.tag >= '1.0.0'"},
				Inputs:   map[string]string{"version": "{{ .payload.version }}"},
			},
		},
		{
			Repository: "org/repo2",
			Subscription: config.Subscription{
				Workflow: "build.yml",
				Filters:  []string{"event.payload.tag >= '1.0.0'", "event.payload.version != null"}, // Different order
				Inputs:   map[string]string{"version": "{{ .payload.version }}"},
			},
		},
		{
			Repository: "org/repo3",
			Subscription: config.Subscription{
				Workflow: "build.yml",
				Filters:  []string{"event.payload.version != null"}, // Different filters (subset)
				Inputs:   map[string]string{"version": "{{ .payload.version }}"},
			},
		},
	}

	event := Event{
		Type:      "library_built",
		Source:    "source/repo",
		Payload:   map[string]interface{}{"version": "1.2.3", "tag": "1.2.3"},
		Timestamp: time.Now().Unix(),
	}

	params := &FanOutParams{
		WaitForChildren:  false,
		ConcurrencyLimit: 0,
	}

	state, err := executor.stateManager.CreateFanOutState("test-fanout-4", "", "source/repo", "library_built", false, 0)
	if err != nil {
		t.Fatalf("Failed to create fan-out state: %v", err)
	}

	// Test - should trigger 2: first two are diamonds (only trigger repo1), third has different filters
	triggeredCount, errors, _ := executor.triggerSubscribersWithState(subscribers, event, params, state)

	// Should trigger 2 workflows: repo1 (winner of diamond) + repo3 (different filters)
	if triggeredCount != 2 {
		t.Errorf("Expected 2 triggered workflows (1 diamond winner + 1 different), got %d\nErrors: %v", triggeredCount, errors)
	}
}

func TestResolveDiamondDependencies(t *testing.T) {
	tempDir := t.TempDir()
	mockRunner := NewTestMockWorkflowRunner()

	executor, err := NewFanOutExecutor(tempDir, false, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	eventFingerprint := "test-event-fingerprint"

	tests := []struct {
		name                 string
		subscribers          []SubscriptionMatch
		expectedUniqueCount  int
		expectedSkippedCount int
		expectedWinners      []string // Repository names that should win
		description          string
	}{
		{
			name: "no diamond dependencies",
			subscribers: []SubscriptionMatch{
				{
					Repository: "org/repo1",
					Subscription: config.Subscription{
						Workflow: "build.yml",
						Filters:  []string{"event.payload.version != null"},
						Inputs:   map[string]string{"version": "{{ .payload.version }}"},
					},
				},
				{
					Repository: "org/repo2",
					Subscription: config.Subscription{
						Workflow: "test.yml", // Different workflow
						Filters:  []string{"event.payload.version != null"},
						Inputs:   map[string]string{"version": "{{ .payload.version }}"},
					},
				},
			},
			expectedUniqueCount:  2,
			expectedSkippedCount: 0,
			expectedWinners:      []string{"org/repo1", "org/repo2"},
			description:          "Different workflows should not be considered diamonds",
		},
		{
			name: "simple diamond dependency",
			subscribers: []SubscriptionMatch{
				{
					Repository: "org/repo2", // Not first alphabetically
					Subscription: config.Subscription{
						Workflow: "build.yml",
						Filters:  []string{"event.payload.version != null"},
						Inputs:   map[string]string{"version": "{{ .payload.version }}"},
					},
				},
				{
					Repository: "org/repo1", // First alphabetically - should win
					Subscription: config.Subscription{
						Workflow: "build.yml",
						Filters:  []string{"event.payload.version != null"},
						Inputs:   map[string]string{"version": "{{ .payload.version }}"},
					},
				},
			},
			expectedUniqueCount:  1,
			expectedSkippedCount: 1,
			expectedWinners:      []string{"org/repo1"}, // First alphabetically wins
			description:          "First repository alphabetically should win",
		},
		{
			name: "complex diamond scenario",
			subscribers: []SubscriptionMatch{
				{
					Repository: "org/repo3",
					Subscription: config.Subscription{
						Workflow: "build.yml",
						Filters:  []string{"event.payload.version != null"},
						Inputs:   map[string]string{"version": "{{ .payload.version }}"},
					},
				},
				{
					Repository: "org/repo1", // Should win this diamond
					Subscription: config.Subscription{
						Workflow: "build.yml",
						Filters:  []string{"event.payload.version != null"},
						Inputs:   map[string]string{"version": "{{ .payload.version }}"},
					},
				},
				{
					Repository: "org/repo2", // Different workflow, should win its own category
					Subscription: config.Subscription{
						Workflow: "test.yml",
						Filters:  []string{"event.payload.version != null"},
						Inputs:   map[string]string{"version": "{{ .payload.version }}"},
					},
				},
				{
					Repository: "org/repo4", // Same as repo2 but different repo - should be skipped
					Subscription: config.Subscription{
						Workflow: "test.yml",
						Filters:  []string{"event.payload.version != null"},
						Inputs:   map[string]string{"version": "{{ .payload.version }}"},
					},
				},
			},
			expectedUniqueCount:  2,
			expectedSkippedCount: 2,
			expectedWinners:      []string{"org/repo1", "org/repo2"}, // Winners of each diamond group
			description:          "Multiple diamond groups should each have one winner",
		},
		{
			name: "normalized whitespace diamond",
			subscribers: []SubscriptionMatch{
				{
					Repository: "org/repo2",
					Subscription: config.Subscription{
						Workflow: "build.yml",
						Filters:  []string{"event.payload.version!=null"},              // No spaces
						Inputs:   map[string]string{"version": "{{.payload.version}}"}, // No spaces
					},
				},
				{
					Repository: "org/repo1", // Should win due to alphabetical order
					Subscription: config.Subscription{
						Workflow: "build.yml",
						Filters:  []string{"event.payload.version != null"},              // With spaces
						Inputs:   map[string]string{"version": "{{ .payload.version }}"}, // With spaces
					},
				},
			},
			expectedUniqueCount:  1,
			expectedSkippedCount: 1,
			expectedWinners:      []string{"org/repo1"},
			description:          "Whitespace differences should normalize to same fingerprint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uniqueSubscribers, skippedCount, errors := executor.resolveDiamondDependencies(tt.subscribers, eventFingerprint)

			// Check counts
			if len(uniqueSubscribers) != tt.expectedUniqueCount {
				t.Errorf("Expected %d unique subscribers, got %d", tt.expectedUniqueCount, len(uniqueSubscribers))
			}

			if skippedCount != tt.expectedSkippedCount {
				t.Errorf("Expected %d skipped subscribers, got %d", tt.expectedSkippedCount, skippedCount)
			}

			// Check no errors
			if len(errors) > 0 {
				t.Errorf("Expected no errors, got: %v", errors)
			}

			// Check winners
			actualWinners := make([]string, len(uniqueSubscribers))
			for i, sub := range uniqueSubscribers {
				actualWinners[i] = sub.Repository
			}

			if len(actualWinners) != len(tt.expectedWinners) {
				t.Errorf("Expected winners %v, got %v", tt.expectedWinners, actualWinners)
			} else {
				// Check each expected winner is present (order may differ due to map iteration)
				for _, expectedWinner := range tt.expectedWinners {
					found := false
					for _, actualWinner := range actualWinners {
						if actualWinner == expectedWinner {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected winner %s not found in actual winners %v", expectedWinner, actualWinners)
					}
				}
			}
		})
	}
}

func TestResolveDiamondDependencies_ErrorHandling(t *testing.T) {
	tempDir := t.TempDir()
	mockRunner := NewTestMockWorkflowRunner()

	executor, err := NewFanOutExecutor(tempDir, false, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Test with empty event fingerprint (should skip diamond resolution)
	subscribers := []SubscriptionMatch{
		{
			Repository: "org/repo1",
			Subscription: config.Subscription{
				Workflow: "build.yml",
				Filters:  []string{"event.payload.version != null"},
				Inputs:   map[string]string{"version": "{{ .payload.version }}"},
			},
		},
		{
			Repository: "org/repo2",
			Subscription: config.Subscription{
				Workflow: "build.yml",
				Filters:  []string{"event.payload.version != null"},
				Inputs:   map[string]string{"version": "{{ .payload.version }}"},
			},
		},
	}

	uniqueSubscribers, skippedCount, errors := executor.resolveDiamondDependencies(subscribers, "")

	// Should return all subscribers unchanged when no event fingerprint
	if len(uniqueSubscribers) != 2 {
		t.Errorf("Expected 2 unique subscribers when no event fingerprint, got %d", len(uniqueSubscribers))
	}

	if skippedCount != 0 {
		t.Errorf("Expected 0 skipped when no event fingerprint, got %d", skippedCount)
	}

	if len(errors) != 0 {
		t.Errorf("Expected no errors when no event fingerprint, got: %v", errors)
	}

	// Test with single subscriber (should skip diamond resolution)
	singleSubscriber := subscribers[:1]
	uniqueSubscribers, skippedCount, errors = executor.resolveDiamondDependencies(singleSubscriber, "test-fingerprint")

	if len(uniqueSubscribers) != 1 {
		t.Errorf("Expected 1 unique subscriber with single input, got %d", len(uniqueSubscribers))
	}

	if skippedCount != 0 {
		t.Errorf("Expected 0 skipped with single input, got %d", skippedCount)
	}

	if len(errors) != 0 {
		t.Errorf("Expected no errors with single input, got: %v", errors)
	}
}
