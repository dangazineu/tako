package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dangazineu/tako/internal/config"
)

func TestFanOutExecutorWithEnhancedEvents(t *testing.T) {
	tempDir := t.TempDir()

	// Create test repository structure
	testRepo1Path := filepath.Join(tempDir, "repos", "test-org", "consumer1", "main")
	testRepo2Path := filepath.Join(tempDir, "repos", "test-org", "consumer2", "main")

	if err := os.MkdirAll(testRepo1Path, 0755); err != nil {
		t.Fatalf("Failed to create test repo1 directory: %v", err)
	}
	if err := os.MkdirAll(testRepo2Path, 0755); err != nil {
		t.Fatalf("Failed to create test repo2 directory: %v", err)
	}

	// Create tako.yml files with enhanced subscriptions
	takoYml1 := `version: "1.0"
workflows:
  update:
    steps:
      - run: echo "update triggered"
subscriptions:
  - artifact: "source-org/library:default"
    events: ["build_completed"]
    workflow: "update"
    schema_version: "^1.0.0"
    filters:
      - event_type == "build_completed"
      - payload.status == "success"
`

	takoYml2 := `version: "1.0"
workflows:
  deploy:
    steps:
      - run: echo "deploy triggered"
subscriptions:
  - artifact: "source-org/library:default"
    events: ["build_completed"]
    workflow: "deploy"
    schema_version: "~1.0.0"
    filters:
      - event_type == "build_completed"
      - payload.environment == "production"
`

	if err := os.WriteFile(filepath.Join(testRepo1Path, "tako.yml"), []byte(takoYml1), 0644); err != nil {
		t.Fatalf("Failed to write tako.yml for repo1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testRepo2Path, "tako.yml"), []byte(takoYml2), 0644); err != nil {
		t.Fatalf("Failed to write tako.yml for repo2: %v", err)
	}

	mockRunner := NewTestMockWorkflowRunner()
	executor, err := NewFanOutExecutor(tempDir, true, mockRunner)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	tests := []struct {
		name              string
		step              config.WorkflowStep
		sourceRepo        string
		expectedTriggered int
		expectSuccess     bool
	}{
		{
			name: "enhanced event with schema validation",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type":     "build_completed",
					"schema_version": "1.0.0",
					"payload": map[string]interface{}{
						"status":      "success",
						"duration":    45.5,
						"commit":      "a1b2c3d4e5f6789012345678901234567890abcd",
						"environment": "production",
					},
				},
			},
			sourceRepo:        "source-org/library",
			expectedTriggered: 2, // Both subscribers should match
			expectSuccess:     true,
		},
		{
			name: "event without schema validation",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type": "build_completed",
					// No schema_version specified, should skip validation
					"payload": map[string]interface{}{
						"status":      "success",
						"environment": "production", // Required by consumer2's filter
					},
				},
			},
			sourceRepo:        "source-org/library",
			expectedTriggered: 2, // Should work without schema validation
			expectSuccess:     true,
		},
		{
			name: "event with filter mismatch",
			step: config.WorkflowStep{
				Uses: "tako/fan-out@v1",
				With: map[string]interface{}{
					"event_type":     "build_completed",
					"schema_version": "1.0.0",
					"payload": map[string]interface{}{
						"status":      "failure", // Won't match consumer1's filter
						"environment": "staging", // Won't match consumer2's filter
					},
				},
			},
			sourceRepo:        "source-org/library",
			expectedTriggered: 0, // No subscribers should match due to filter mismatch
			expectSuccess:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.Execute(tt.step, tt.sourceRepo)

			if tt.expectSuccess {
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

			if tt.expectSuccess {
				if !result.EventEmitted {
					t.Errorf("Expected event to be emitted")
				}
			}
		})
	}
}

func TestEventModelSchemaValidation(t *testing.T) {
	validator := NewEventValidator()

	// Register common schemas
	err := RegisterCommonSchemas(validator)
	if err != nil {
		t.Fatalf("Failed to register common schemas: %v", err)
	}

	tests := []struct {
		name        string
		event       EnhancedEvent
		expectError bool
	}{
		{
			name: "valid build_completed event",
			event: EnhancedEvent{
				Type:   "build_completed",
				Schema: "build_completed@1.0.0",
				Payload: map[string]interface{}{
					"status":   "success",
					"duration": 30.5,
					"commit":   "1234567890abcdef1234567890abcdef12345678",
				},
			},
			expectError: false,
		},
		{
			name: "invalid build_completed event - missing required field",
			event: EnhancedEvent{
				Type:   "build_completed",
				Schema: "build_completed@1.0.0",
				Payload: map[string]interface{}{
					"duration": 30.5,
					// Missing required "status"
				},
			},
			expectError: true,
		},
		{
			name: "invalid build_completed event - invalid status enum",
			event: EnhancedEvent{
				Type:   "build_completed",
				Schema: "build_completed@1.0.0",
				Payload: map[string]interface{}{
					"status": "unknown", // Not in enum
				},
			},
			expectError: true,
		},
		{
			name: "invalid build_completed event - invalid commit format",
			event: EnhancedEvent{
				Type:   "build_completed",
				Schema: "build_completed@1.0.0",
				Payload: map[string]interface{}{
					"status": "success",
					"commit": "invalid", // Wrong format
				},
			},
			expectError: true,
		},
		{
			name: "valid deployment_started event",
			event: EnhancedEvent{
				Type:   "deployment_started",
				Schema: "deployment_started@1.0.0",
				Payload: map[string]interface{}{
					"environment": "production",
					"version":     "v1.2.3",
					"deployer":    "user123",
				},
			},
			expectError: false,
		},
		{
			name: "valid test_results event",
			event: EnhancedEvent{
				Type:   "test_results",
				Schema: "test_results@1.0.0",
				Payload: map[string]interface{}{
					"total":    100.0,
					"passed":   95.0,
					"failed":   5.0,
					"coverage": 87.5,
					"suite":    "unit-tests",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateEvent(tt.event)
			if tt.expectError {
				if err == nil {
					t.Error("Expected validation error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected validation error: %v", err)
				}
			}
		})
	}
}

func TestEventBuilderWithCommonSchemas(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		builder   func() EnhancedEvent
		validate  func(t *testing.T, event EnhancedEvent)
	}{
		{
			name:      "build_completed event",
			eventType: "build_completed",
			builder: func() EnhancedEvent {
				return NewEventBuilder("build_completed").
					WithSchema("build_completed@1.0.0").
					WithSource("ci/cd-pipeline").
					WithProperty("status", "success").
					WithProperty("duration", 120.5).
					WithProperty("commit", "abcdef1234567890abcdef1234567890abcdef12").
					WithProperty("artifacts", []interface{}{"binary", "docs"}).
					WithCorrelation("build-456").
					Build()
			},
			validate: func(t *testing.T, event EnhancedEvent) {
				if event.Type != "build_completed" {
					t.Errorf("Expected type 'build_completed', got '%s'", event.Type)
				}
				if event.Payload["status"] != "success" {
					t.Errorf("Expected status 'success', got '%v'", event.Payload["status"])
				}
				if event.Payload["duration"] != 120.5 {
					t.Errorf("Expected duration 120.5, got %v", event.Payload["duration"])
				}
			},
		},
		{
			name:      "deployment_started event",
			eventType: "deployment_started",
			builder: func() EnhancedEvent {
				return NewEventBuilder("deployment_started").
					WithSchema("deployment_started@1.0.0").
					WithSource("deploy-service").
					WithProperty("environment", "staging").
					WithProperty("version", "v2.1.0").
					WithProperty("deployer", "auto-deployer").
					WithTrace("trace-789").
					Build()
			},
			validate: func(t *testing.T, event EnhancedEvent) {
				if event.Type != "deployment_started" {
					t.Errorf("Expected type 'deployment_started', got '%s'", event.Type)
				}
				if event.Payload["environment"] != "staging" {
					t.Errorf("Expected environment 'staging', got '%v'", event.Payload["environment"])
				}
			},
		},
		{
			name:      "test_results event",
			eventType: "test_results",
			builder: func() EnhancedEvent {
				return NewEventBuilder("test_results").
					WithSchema("test_results@1.0.0").
					WithSource("test-runner").
					WithProperty("total", 250.0).
					WithProperty("passed", 240.0).
					WithProperty("failed", 10.0).
					WithProperty("coverage", 92.3).
					WithProperty("suite", "integration-tests").
					Build()
			},
			validate: func(t *testing.T, event EnhancedEvent) {
				if event.Type != "test_results" {
					t.Errorf("Expected type 'test_results', got '%s'", event.Type)
				}
				if event.Payload["total"] != 250.0 {
					t.Errorf("Expected total 250, got %v", event.Payload["total"])
				}
			},
		},
	}

	// Create validator with common schemas
	validator := NewEventValidator()
	err := RegisterCommonSchemas(validator)
	if err != nil {
		t.Fatalf("Failed to register common schemas: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := tt.builder()
			tt.validate(t, event)

			// Validate against schema
			err := validator.ValidateEvent(event)
			if err != nil {
				t.Errorf("Event validation failed: %v", err)
			}

			// Test serialization/deserialization
			data, err := SerializeEvent(event)
			if err != nil {
				t.Errorf("Failed to serialize event: %v", err)
			}

			deserializedEvent, err := DeserializeEvent(data)
			if err != nil {
				t.Errorf("Failed to deserialize event: %v", err)
			}

			if deserializedEvent.Type != event.Type {
				t.Errorf("Deserialized type mismatch: expected %s, got %s", event.Type, deserializedEvent.Type)
			}
		})
	}
}

func TestEventModelBackwardCompatibility(t *testing.T) {
	// Test that enhanced events can be converted to legacy events and vice versa
	legacyEvent := Event{
		Type:          "legacy_build",
		SchemaVersion: "1.5.0",
		Payload: map[string]interface{}{
			"status": "success",
			"branch": "main",
		},
		Source:    "legacy-ci",
		Timestamp: time.Now().Unix(),
	}

	// Convert to enhanced
	enhancedEvent := ConvertLegacyEvent(legacyEvent)

	if enhancedEvent.Type != legacyEvent.Type {
		t.Errorf("Type mismatch after conversion: expected %s, got %s", legacyEvent.Type, enhancedEvent.Type)
	}
	if enhancedEvent.Schema != "legacy_build@1.5.0" {
		t.Errorf("Schema mismatch: expected 'legacy_build@1.5.0', got '%s'", enhancedEvent.Schema)
	}
	if enhancedEvent.Metadata.Source != legacyEvent.Source {
		t.Errorf("Source mismatch: expected %s, got %s", legacyEvent.Source, enhancedEvent.Metadata.Source)
	}

	// Convert back to legacy
	convertedBack := enhancedEvent.ToLegacyEvent()

	if convertedBack.Type != legacyEvent.Type {
		t.Errorf("Type mismatch after round-trip: expected %s, got %s", legacyEvent.Type, convertedBack.Type)
	}
	if convertedBack.SchemaVersion != legacyEvent.SchemaVersion {
		t.Errorf("Schema version mismatch: expected %s, got %s", legacyEvent.SchemaVersion, convertedBack.SchemaVersion)
	}
	if convertedBack.Source != legacyEvent.Source {
		t.Errorf("Source mismatch: expected %s, got %s", legacyEvent.Source, convertedBack.Source)
	}
	if convertedBack.Payload["status"] != "success" {
		t.Errorf("Payload mismatch: expected 'success', got '%v'", convertedBack.Payload["status"])
	}
}

func TestEventValidationPerformance(t *testing.T) {
	validator := NewEventValidator()
	err := RegisterCommonSchemas(validator)
	if err != nil {
		t.Fatalf("Failed to register common schemas: %v", err)
	}

	// Create a valid test event
	event := EnhancedEvent{
		Type:   "build_completed",
		Schema: "build_completed@1.0.0",
		Payload: map[string]interface{}{
			"status":   "success",
			"duration": 45.5,
			"commit":   "a1b2c3d4e5f6789012345678901234567890abcd",
		},
	}

	// Validate multiple times to test performance
	iterations := 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		err := validator.ValidateEvent(event)
		if err != nil {
			t.Fatalf("Validation failed on iteration %d: %v", i, err)
		}
	}

	elapsed := time.Since(start)
	avgPerValidation := elapsed / time.Duration(iterations)

	// Performance should be reasonable (less than 1ms per validation)
	if avgPerValidation > time.Millisecond {
		t.Errorf("Validation performance too slow: %v per validation", avgPerValidation)
	}

	t.Logf("Validation performance: %v per validation (%d iterations in %v)", avgPerValidation, iterations, elapsed)
}
