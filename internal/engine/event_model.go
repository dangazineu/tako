package engine

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// EventSchema defines the structure and validation rules for event payloads.
type EventSchema struct {
	Version     string                 `json:"version"`
	Type        string                 `json:"type"`
	Description string                 `json:"description,omitempty"`
	Properties  map[string]PropertyDef `json:"properties"`
	Required    []string               `json:"required,omitempty"`
}

// PropertyDef defines validation rules for event payload properties.
type PropertyDef struct {
	Type        string      `json:"type"` // string, number, boolean, object, array
	Description string      `json:"description,omitempty"`
	Pattern     string      `json:"pattern,omitempty"`   // regex for string validation
	MinLength   *int        `json:"minLength,omitempty"` // minimum string length
	MaxLength   *int        `json:"maxLength,omitempty"` // maximum string length
	Minimum     *float64    `json:"minimum,omitempty"`   // minimum numeric value
	Maximum     *float64    `json:"maximum,omitempty"`   // maximum numeric value
	Enum        []string    `json:"enum,omitempty"`      // allowed values
	Default     interface{} `json:"default,omitempty"`   // default value
}

// EventMetadata contains metadata about an event emission.
type EventMetadata struct {
	ID            string            `json:"id"`
	Timestamp     time.Time         `json:"timestamp"`
	Source        string            `json:"source"`
	SourceVersion string            `json:"source_version,omitempty"`
	Correlation   string            `json:"correlation,omitempty"`
	Trace         string            `json:"trace,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

// EnhancedEvent represents an event with schema validation and metadata.
type EnhancedEvent struct {
	Type     string                 `json:"type"`
	Schema   string                 `json:"schema,omitempty"`
	Payload  map[string]interface{} `json:"payload"`
	Metadata EventMetadata          `json:"metadata"`
}

// EventValidator handles event schema validation and payload processing.
type EventValidator struct {
	schemas map[string]EventSchema
}

// NewEventValidator creates a new event validator.
func NewEventValidator() *EventValidator {
	return &EventValidator{
		schemas: make(map[string]EventSchema),
	}
}

// RegisterSchema registers an event schema for validation.
func (ev *EventValidator) RegisterSchema(schema EventSchema) error {
	if schema.Type == "" {
		return fmt.Errorf("schema type cannot be empty")
	}
	if schema.Version == "" {
		return fmt.Errorf("schema version cannot be empty")
	}

	schemaKey := fmt.Sprintf("%s@%s", schema.Type, schema.Version)
	ev.schemas[schemaKey] = schema
	return nil
}

// ValidateEvent validates an event against its registered schema.
func (ev *EventValidator) ValidateEvent(event EnhancedEvent) error {
	if event.Schema == "" {
		// No schema specified, skip validation
		return nil
	}

	schema, exists := ev.schemas[event.Schema]
	if !exists {
		return fmt.Errorf("schema not found: %s", event.Schema)
	}

	// Validate required properties
	for _, required := range schema.Required {
		if _, exists := event.Payload[required]; !exists {
			return fmt.Errorf("required property missing: %s", required)
		}
	}

	// Validate each property
	for key, value := range event.Payload {
		propDef, exists := schema.Properties[key]
		if !exists {
			// Property not defined in schema, allow it (permissive validation)
			continue
		}

		if err := ev.validateProperty(value, propDef); err != nil {
			return fmt.Errorf("property validation failed for '%s': %v", key, err)
		}
	}

	return nil
}

// validateProperty validates a single property against its definition.
func (ev *EventValidator) validateProperty(value interface{}, propDef PropertyDef) error {
	// Enum validation first (applies to all types)
	if len(propDef.Enum) > 0 {
		strVal := fmt.Sprintf("%v", value)
		found := false
		for _, allowed := range propDef.Enum {
			if strVal == allowed {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("value '%v' not in allowed enum values: %v", value, propDef.Enum)
		}
	}

	// Type validation
	switch propDef.Type {
	case "string":
		strVal, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
		return ev.validateStringProperty(strVal, propDef)
	case "number":
		var numVal float64
		switch v := value.(type) {
		case float64:
			numVal = v
		case int:
			numVal = float64(v)
		case int64:
			numVal = float64(v)
		default:
			return fmt.Errorf("expected number, got %T", value)
		}
		return ev.validateNumberProperty(numVal, propDef)
	case "boolean":
		_, ok := value.(bool)
		if !ok {
			return fmt.Errorf("expected boolean, got %T", value)
		}
	case "object":
		_, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected object, got %T", value)
		}
	case "array":
		_, ok := value.([]interface{})
		if !ok {
			return fmt.Errorf("expected array, got %T", value)
		}
	default:
		return fmt.Errorf("unsupported property type: %s", propDef.Type)
	}

	return nil
}

// validateStringProperty validates string-specific constraints.
func (ev *EventValidator) validateStringProperty(value string, propDef PropertyDef) error {
	// Length validation
	if propDef.MinLength != nil && len(value) < *propDef.MinLength {
		return fmt.Errorf("string length %d is less than minimum %d", len(value), *propDef.MinLength)
	}
	if propDef.MaxLength != nil && len(value) > *propDef.MaxLength {
		return fmt.Errorf("string length %d exceeds maximum %d", len(value), *propDef.MaxLength)
	}

	// Pattern validation
	if propDef.Pattern != "" {
		matched, err := regexp.MatchString(propDef.Pattern, value)
		if err != nil {
			return fmt.Errorf("invalid regex pattern '%s': %v", propDef.Pattern, err)
		}
		if !matched {
			return fmt.Errorf("string '%s' does not match pattern '%s'", value, propDef.Pattern)
		}
	}

	return nil
}

// validateNumberProperty validates number-specific constraints.
func (ev *EventValidator) validateNumberProperty(value float64, propDef PropertyDef) error {
	// Range validation
	if propDef.Minimum != nil && value < *propDef.Minimum {
		return fmt.Errorf("value %f is less than minimum %f", value, *propDef.Minimum)
	}
	if propDef.Maximum != nil && value > *propDef.Maximum {
		return fmt.Errorf("value %f exceeds maximum %f", value, *propDef.Maximum)
	}

	return nil
}

// ApplyDefaults applies default values to event payload based on schema.
func (ev *EventValidator) ApplyDefaults(event *EnhancedEvent) error {
	if event.Schema == "" {
		return nil
	}

	schema, exists := ev.schemas[event.Schema]
	if !exists {
		return fmt.Errorf("schema not found: %s", event.Schema)
	}

	if event.Payload == nil {
		event.Payload = make(map[string]interface{})
	}

	// Apply default values for missing properties
	for key, propDef := range schema.Properties {
		if _, exists := event.Payload[key]; !exists && propDef.Default != nil {
			event.Payload[key] = propDef.Default
		}
	}

	return nil
}

// ConvertLegacyEvent converts a legacy Event to an EnhancedEvent.
func ConvertLegacyEvent(legacyEvent Event) EnhancedEvent {
	return EnhancedEvent{
		Type:    legacyEvent.Type,
		Schema:  fmt.Sprintf("%s@%s", legacyEvent.Type, legacyEvent.SchemaVersion),
		Payload: legacyEvent.Payload,
		Metadata: EventMetadata{
			ID:        fmt.Sprintf("evt_%d_%s", legacyEvent.Timestamp, legacyEvent.Type),
			Timestamp: time.Unix(legacyEvent.Timestamp, 0),
			Source:    legacyEvent.Source,
		},
	}
}

// ToLegacyEvent converts an EnhancedEvent to a legacy Event for backward compatibility.
func (e EnhancedEvent) ToLegacyEvent() Event {
	schemaVersion := ""
	if e.Schema != "" {
		// Extract version from "type@version" format
		parts := regexp.MustCompile(`@(.+)$`).FindStringSubmatch(e.Schema)
		if len(parts) > 1 {
			schemaVersion = parts[1]
		}
	}

	return Event{
		Type:          e.Type,
		SchemaVersion: schemaVersion,
		Payload:       e.Payload,
		Source:        e.Metadata.Source,
		Timestamp:     e.Metadata.Timestamp.Unix(),
	}
}

// EventBuilder provides a fluent interface for building events.
type EventBuilder struct {
	event EnhancedEvent
}

// NewEventBuilder creates a new event builder.
func NewEventBuilder(eventType string) *EventBuilder {
	return &EventBuilder{
		event: EnhancedEvent{
			Type:    eventType,
			Payload: make(map[string]interface{}),
			Metadata: EventMetadata{
				ID:        generateEventID(),
				Timestamp: time.Now(),
				Headers:   make(map[string]string),
			},
		},
	}
}

// WithSchema sets the event schema.
func (eb *EventBuilder) WithSchema(schema string) *EventBuilder {
	eb.event.Schema = schema
	return eb
}

// WithSource sets the event source.
func (eb *EventBuilder) WithSource(source string) *EventBuilder {
	eb.event.Metadata.Source = source
	return eb
}

// WithPayload sets the entire payload.
func (eb *EventBuilder) WithPayload(payload map[string]interface{}) *EventBuilder {
	eb.event.Payload = payload
	return eb
}

// WithProperty sets a single payload property.
func (eb *EventBuilder) WithProperty(key string, value interface{}) *EventBuilder {
	eb.event.Payload[key] = value
	return eb
}

// WithCorrelation sets the correlation ID.
func (eb *EventBuilder) WithCorrelation(correlationID string) *EventBuilder {
	eb.event.Metadata.Correlation = correlationID
	return eb
}

// WithTrace sets the trace ID.
func (eb *EventBuilder) WithTrace(traceID string) *EventBuilder {
	eb.event.Metadata.Trace = traceID
	return eb
}

// WithHeader sets a metadata header.
func (eb *EventBuilder) WithHeader(key, value string) *EventBuilder {
	eb.event.Metadata.Headers[key] = value
	return eb
}

// Build returns the constructed event.
func (eb *EventBuilder) Build() EnhancedEvent {
	return eb.event
}

// generateEventID generates a unique event ID.
func generateEventID() string {
	return fmt.Sprintf("evt_%d_%d", time.Now().UnixNano(), time.Now().Nanosecond()%1000000)
}

// CommonEventSchemas contains frequently used event schemas.
var CommonEventSchemas = map[string]EventSchema{
	"build_completed": {
		Version:     "1.0.0",
		Type:        "build_completed",
		Description: "Emitted when a build process completes",
		Properties: map[string]PropertyDef{
			"status": {
				Type:        "string",
				Description: "Build completion status",
				Enum:        []string{"success", "failure", "cancelled"},
			},
			"duration": {
				Type:        "number",
				Description: "Build duration in seconds",
				Minimum:     func() *float64 { v := 0.0; return &v }(),
			},
			"commit": {
				Type:        "string",
				Description: "Git commit hash",
				Pattern:     "^[a-f0-9]{40}$",
				MinLength:   func() *int { v := 40; return &v }(),
				MaxLength:   func() *int { v := 40; return &v }(),
			},
			"artifacts": {
				Type:        "array",
				Description: "List of generated artifacts",
			},
		},
		Required: []string{"status"},
	},
	"deployment_started": {
		Version:     "1.0.0",
		Type:        "deployment_started",
		Description: "Emitted when a deployment begins",
		Properties: map[string]PropertyDef{
			"environment": {
				Type:        "string",
				Description: "Target deployment environment",
				Enum:        []string{"development", "staging", "production"},
			},
			"version": {
				Type:        "string",
				Description: "Version being deployed",
				Pattern:     `^v?\d+\.\d+\.\d+`,
			},
			"deployer": {
				Type:        "string",
				Description: "User or system initiating deployment",
				MinLength:   func() *int { v := 1; return &v }(),
			},
		},
		Required: []string{"environment", "version"},
	},
	"test_results": {
		Version:     "1.0.0",
		Type:        "test_results",
		Description: "Emitted when test execution completes",
		Properties: map[string]PropertyDef{
			"total": {
				Type:        "number",
				Description: "Total number of tests executed",
				Minimum:     func() *float64 { v := 0.0; return &v }(),
			},
			"passed": {
				Type:        "number",
				Description: "Number of tests that passed",
				Minimum:     func() *float64 { v := 0.0; return &v }(),
			},
			"failed": {
				Type:        "number",
				Description: "Number of tests that failed",
				Minimum:     func() *float64 { v := 0.0; return &v }(),
			},
			"coverage": {
				Type:        "number",
				Description: "Test coverage percentage",
				Minimum:     func() *float64 { v := 0.0; return &v }(),
				Maximum:     func() *float64 { v := 100.0; return &v }(),
			},
			"suite": {
				Type:        "string",
				Description: "Test suite identifier",
			},
		},
		Required: []string{"total", "passed", "failed"},
	},
}

// RegisterCommonSchemas registers all common event schemas with a validator.
func RegisterCommonSchemas(validator *EventValidator) error {
	for _, schema := range CommonEventSchemas {
		if err := validator.RegisterSchema(schema); err != nil {
			return fmt.Errorf("failed to register schema %s@%s: %v", schema.Type, schema.Version, err)
		}
	}
	return nil
}

// SerializeEvent serializes an event to JSON.
func SerializeEvent(event EnhancedEvent) ([]byte, error) {
	return json.MarshalIndent(event, "", "  ")
}

// DeserializeEvent deserializes an event from JSON.
func DeserializeEvent(data []byte) (EnhancedEvent, error) {
	var event EnhancedEvent
	err := json.Unmarshal(data, &event)
	return event, err
}
