package engine

import (
	"testing"
	"time"
)

func TestNewEventValidator(t *testing.T) {
	validator := NewEventValidator()
	if validator == nil {
		t.Fatal("Expected validator to be created")
	}
	if len(validator.schemas) != 0 {
		t.Errorf("Expected empty schemas map, got %d items", len(validator.schemas))
	}
}

func TestRegisterSchema(t *testing.T) {
	validator := NewEventValidator()

	schema := EventSchema{
		Version: "1.0.0",
		Type:    "test_event",
		Properties: map[string]PropertyDef{
			"name": {Type: "string"},
		},
	}

	err := validator.RegisterSchema(schema)
	if err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	// Verify schema was registered
	if len(validator.schemas) != 1 {
		t.Errorf("Expected 1 schema, got %d", len(validator.schemas))
	}

	// Test error cases
	tests := []struct {
		name   string
		schema EventSchema
	}{
		{
			name: "empty type",
			schema: EventSchema{
				Version: "1.0.0",
				Type:    "",
			},
		},
		{
			name: "empty version",
			schema: EventSchema{
				Version: "",
				Type:    "test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.RegisterSchema(tt.schema)
			if err == nil {
				t.Error("Expected error for invalid schema")
			}
		})
	}
}

func TestValidateEvent(t *testing.T) {
	validator := NewEventValidator()

	// Register test schema
	schema := EventSchema{
		Version:     "1.0.0",
		Type:        "test_event",
		Description: "Test event for validation",
		Properties: map[string]PropertyDef{
			"name": {
				Type:        "string",
				Description: "Event name",
				MinLength:   intPtr(1),
				MaxLength:   intPtr(50),
			},
			"count": {
				Type:        "number",
				Description: "Event count",
				Minimum:     floatPtr(0),
				Maximum:     floatPtr(100),
			},
			"enabled": {
				Type:        "boolean",
				Description: "Event enabled flag",
			},
			"status": {
				Type:        "string",
				Description: "Event status",
				Enum:        []string{"active", "inactive", "pending"},
			},
			"tags": {
				Type:        "array",
				Description: "Event tags",
			},
			"metadata": {
				Type:        "object",
				Description: "Event metadata",
			},
		},
		Required: []string{"name", "count"},
	}

	err := validator.RegisterSchema(schema)
	if err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	tests := []struct {
		name        string
		event       EnhancedEvent
		expectError bool
	}{
		{
			name: "valid event",
			event: EnhancedEvent{
				Type:   "test_event",
				Schema: "test_event@1.0.0",
				Payload: map[string]interface{}{
					"name":     "test",
					"count":    42.0,
					"enabled":  true,
					"status":   "active",
					"tags":     []interface{}{"tag1", "tag2"},
					"metadata": map[string]interface{}{"key": "value"},
				},
			},
			expectError: false,
		},
		{
			name: "no schema specified",
			event: EnhancedEvent{
				Type:    "test_event",
				Schema:  "",
				Payload: map[string]interface{}{},
			},
			expectError: false, // Should skip validation
		},
		{
			name: "unknown schema",
			event: EnhancedEvent{
				Type:   "test_event",
				Schema: "unknown@1.0.0",
				Payload: map[string]interface{}{
					"name": "test",
				},
			},
			expectError: true,
		},
		{
			name: "missing required property",
			event: EnhancedEvent{
				Type:   "test_event",
				Schema: "test_event@1.0.0",
				Payload: map[string]interface{}{
					"name": "test",
					// Missing required "count"
				},
			},
			expectError: true,
		},
		{
			name: "invalid string type",
			event: EnhancedEvent{
				Type:   "test_event",
				Schema: "test_event@1.0.0",
				Payload: map[string]interface{}{
					"name":  123, // Should be string
					"count": 42.0,
				},
			},
			expectError: true,
		},
		{
			name: "invalid number type",
			event: EnhancedEvent{
				Type:   "test_event",
				Schema: "test_event@1.0.0",
				Payload: map[string]interface{}{
					"name":  "test",
					"count": "not a number", // Should be number
				},
			},
			expectError: true,
		},
		{
			name: "string too short",
			event: EnhancedEvent{
				Type:   "test_event",
				Schema: "test_event@1.0.0",
				Payload: map[string]interface{}{
					"name":  "", // Too short (min length 1)
					"count": 42.0,
				},
			},
			expectError: true,
		},
		{
			name: "number out of range",
			event: EnhancedEvent{
				Type:   "test_event",
				Schema: "test_event@1.0.0",
				Payload: map[string]interface{}{
					"name":  "test",
					"count": 150.0, // Too high (max 100)
				},
			},
			expectError: true,
		},
		{
			name: "invalid enum value",
			event: EnhancedEvent{
				Type:   "test_event",
				Schema: "test_event@1.0.0",
				Payload: map[string]interface{}{
					"name":   "test",
					"count":  42.0,
					"status": "invalid", // Not in enum
				},
			},
			expectError: true,
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

func TestApplyDefaults(t *testing.T) {
	validator := NewEventValidator()

	// Register schema with defaults
	schema := EventSchema{
		Version: "1.0.0",
		Type:    "test_event",
		Properties: map[string]PropertyDef{
			"name": {
				Type:    "string",
				Default: "default_name",
			},
			"count": {
				Type:    "number",
				Default: 10.0,
			},
			"enabled": {
				Type:    "boolean",
				Default: true,
			},
		},
	}

	err := validator.RegisterSchema(schema)
	if err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	tests := []struct {
		name     string
		event    EnhancedEvent
		expected map[string]interface{}
	}{
		{
			name: "apply all defaults",
			event: EnhancedEvent{
				Type:    "test_event",
				Schema:  "test_event@1.0.0",
				Payload: map[string]interface{}{
					// Empty payload
				},
			},
			expected: map[string]interface{}{
				"name":    "default_name",
				"count":   10.0,
				"enabled": true,
			},
		},
		{
			name: "partial defaults",
			event: EnhancedEvent{
				Type:   "test_event",
				Schema: "test_event@1.0.0",
				Payload: map[string]interface{}{
					"name": "custom_name", // Override default
				},
			},
			expected: map[string]interface{}{
				"name":    "custom_name",
				"count":   10.0,
				"enabled": true,
			},
		},
		{
			name: "no schema",
			event: EnhancedEvent{
				Type:   "test_event",
				Schema: "",
				Payload: map[string]interface{}{
					"existing": "value",
				},
			},
			expected: map[string]interface{}{
				"existing": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ApplyDefaults(&tt.event)
			if err != nil {
				t.Errorf("Unexpected error applying defaults: %v", err)
			}

			for key, expectedValue := range tt.expected {
				if actualValue, exists := tt.event.Payload[key]; !exists {
					t.Errorf("Expected key '%s' to exist in payload", key)
				} else if actualValue != expectedValue {
					t.Errorf("Expected payload[%s] = %v, got %v", key, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestEventBuilder(t *testing.T) {
	eventType := "test_event"
	builder := NewEventBuilder(eventType)

	if builder == nil {
		t.Fatal("Expected builder to be created")
	}

	event := builder.
		WithSchema("test_event@1.0.0").
		WithSource("test-source").
		WithPayload(map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		}).
		WithProperty("key3", true).
		WithCorrelation("corr-123").
		WithTrace("trace-456").
		WithHeader("X-Custom", "header-value").
		Build()

	if event.Type != eventType {
		t.Errorf("Expected type %s, got %s", eventType, event.Type)
	}
	if event.Schema != "test_event@1.0.0" {
		t.Errorf("Expected schema 'test_event@1.0.0', got '%s'", event.Schema)
	}
	if event.Metadata.Source != "test-source" {
		t.Errorf("Expected source 'test-source', got '%s'", event.Metadata.Source)
	}
	if event.Metadata.Correlation != "corr-123" {
		t.Errorf("Expected correlation 'corr-123', got '%s'", event.Metadata.Correlation)
	}
	if event.Metadata.Trace != "trace-456" {
		t.Errorf("Expected trace 'trace-456', got '%s'", event.Metadata.Trace)
	}
	if event.Metadata.Headers["X-Custom"] != "header-value" {
		t.Errorf("Expected header 'header-value', got '%s'", event.Metadata.Headers["X-Custom"])
	}

	// Check payload
	if event.Payload["key1"] != "value1" {
		t.Errorf("Expected payload key1 'value1', got '%v'", event.Payload["key1"])
	}
	if event.Payload["key2"] != 42 {
		t.Errorf("Expected payload key2 42, got %v", event.Payload["key2"])
	}
	if event.Payload["key3"] != true {
		t.Errorf("Expected payload key3 true, got %v", event.Payload["key3"])
	}

	// Check metadata auto-generated fields
	if event.Metadata.ID == "" {
		t.Error("Expected event ID to be generated")
	}
	if event.Metadata.Timestamp.IsZero() {
		t.Error("Expected event timestamp to be set")
	}
}

func TestConvertLegacyEvent(t *testing.T) {
	legacyEvent := Event{
		Type:          "legacy_event",
		SchemaVersion: "2.0.0",
		Payload: map[string]interface{}{
			"data": "test",
		},
		Source:    "legacy-source",
		Timestamp: 1640995200, // 2022-01-01 00:00:00 UTC
	}

	enhancedEvent := ConvertLegacyEvent(legacyEvent)

	if enhancedEvent.Type != "legacy_event" {
		t.Errorf("Expected type 'legacy_event', got '%s'", enhancedEvent.Type)
	}
	if enhancedEvent.Schema != "legacy_event@2.0.0" {
		t.Errorf("Expected schema 'legacy_event@2.0.0', got '%s'", enhancedEvent.Schema)
	}
	if enhancedEvent.Metadata.Source != "legacy-source" {
		t.Errorf("Expected source 'legacy-source', got '%s'", enhancedEvent.Metadata.Source)
	}
	if enhancedEvent.Metadata.Timestamp != time.Unix(1640995200, 0) {
		t.Errorf("Expected timestamp %v, got %v", time.Unix(1640995200, 0), enhancedEvent.Metadata.Timestamp)
	}
	if enhancedEvent.Payload["data"] != "test" {
		t.Errorf("Expected payload data 'test', got '%v'", enhancedEvent.Payload["data"])
	}
}

func TestToLegacyEvent(t *testing.T) {
	enhancedEvent := EnhancedEvent{
		Type:   "enhanced_event",
		Schema: "enhanced_event@3.1.0",
		Payload: map[string]interface{}{
			"data": "test",
		},
		Metadata: EventMetadata{
			ID:        "evt-123",
			Timestamp: time.Unix(1640995200, 0),
			Source:    "enhanced-source",
		},
	}

	legacyEvent := enhancedEvent.ToLegacyEvent()

	if legacyEvent.Type != "enhanced_event" {
		t.Errorf("Expected type 'enhanced_event', got '%s'", legacyEvent.Type)
	}
	if legacyEvent.SchemaVersion != "3.1.0" {
		t.Errorf("Expected schema version '3.1.0', got '%s'", legacyEvent.SchemaVersion)
	}
	if legacyEvent.Source != "enhanced-source" {
		t.Errorf("Expected source 'enhanced-source', got '%s'", legacyEvent.Source)
	}
	if legacyEvent.Timestamp != 1640995200 {
		t.Errorf("Expected timestamp 1640995200, got %d", legacyEvent.Timestamp)
	}
	if legacyEvent.Payload["data"] != "test" {
		t.Errorf("Expected payload data 'test', got '%v'", legacyEvent.Payload["data"])
	}
}

func TestRegisterCommonSchemas(t *testing.T) {
	validator := NewEventValidator()

	err := RegisterCommonSchemas(validator)
	if err != nil {
		t.Fatalf("Failed to register common schemas: %v", err)
	}

	// Check that schemas were registered
	expectedSchemas := []string{
		"build_completed@1.0.0",
		"deployment_started@1.0.0",
		"test_results@1.0.0",
	}

	for _, schemaKey := range expectedSchemas {
		if _, exists := validator.schemas[schemaKey]; !exists {
			t.Errorf("Expected schema '%s' to be registered", schemaKey)
		}
	}

	// Test validation with common schemas
	buildEvent := EnhancedEvent{
		Type:   "build_completed",
		Schema: "build_completed@1.0.0",
		Payload: map[string]interface{}{
			"status":   "success",
			"duration": 45.5,
			"commit":   "a1b2c3d4e5f6789012345678901234567890abcd",
		},
	}

	err = validator.ValidateEvent(buildEvent)
	if err != nil {
		t.Errorf("Build event validation failed: %v", err)
	}
}

func TestSerializeDeserializeEvent(t *testing.T) {
	event := EnhancedEvent{
		Type:   "test_event",
		Schema: "test_event@1.0.0",
		Payload: map[string]interface{}{
			"name":  "test",
			"count": 42.0,
		},
		Metadata: EventMetadata{
			ID:        "evt-123",
			Timestamp: time.Unix(1640995200, 0),
			Source:    "test-source",
			Headers: map[string]string{
				"X-Custom": "value",
			},
		},
	}

	// Serialize
	data, err := SerializeEvent(event)
	if err != nil {
		t.Fatalf("Failed to serialize event: %v", err)
	}

	// Deserialize
	deserializedEvent, err := DeserializeEvent(data)
	if err != nil {
		t.Fatalf("Failed to deserialize event: %v", err)
	}

	// Compare
	if deserializedEvent.Type != event.Type {
		t.Errorf("Type mismatch: expected %s, got %s", event.Type, deserializedEvent.Type)
	}
	if deserializedEvent.Schema != event.Schema {
		t.Errorf("Schema mismatch: expected %s, got %s", event.Schema, deserializedEvent.Schema)
	}
	if deserializedEvent.Metadata.Source != event.Metadata.Source {
		t.Errorf("Source mismatch: expected %s, got %s", event.Metadata.Source, deserializedEvent.Metadata.Source)
	}
}

// Helper functions for test data.
func intPtr(i int) *int {
	return &i
}

func floatPtr(f float64) *float64 {
	return &f
}
