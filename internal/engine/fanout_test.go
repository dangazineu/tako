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
  - artifact: "source-org/library:default"
    events: ["library_built"]
    workflow: "update"
`

	takoYml2 := `version: "1.0"
workflows:
  build:
    steps:
      - run: echo "build triggered"
subscriptions:
  - artifact: "source-org/library:default"
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
