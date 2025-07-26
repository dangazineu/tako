package config

import (
	"fmt"
	"regexp"
)

// Event represents an event that can be emitted by a workflow step
type Event struct {
	Type          string            `yaml:"type"`
	SchemaVersion string            `yaml:"schema_version,omitempty"`
	Payload       map[string]string `yaml:"payload,omitempty"`
}

// EventProduction represents the events section in a produces block
type EventProduction struct {
	Events []Event `yaml:"events,omitempty"`
}

// validateEventType validates that event types follow the naming conventions
func validateEventType(eventType string) error {
	// Event types should be snake_case and not empty
	if eventType == "" {
		return fmt.Errorf("event type cannot be empty")
	}

	// Basic validation - only lowercase letters, numbers, and underscores
	matched, err := regexp.MatchString("^[a-z][a-z0-9_]*$", eventType)
	if err != nil {
		return fmt.Errorf("error validating event type: %w", err)
	}
	if !matched {
		return fmt.Errorf("event type '%s' must be snake_case (lowercase letters, numbers, underscores only)", eventType)
	}

	return nil
}

// validateSchemaVersion validates semantic version format if provided
func validateSchemaVersion(version string) error {
	if version == "" {
		return nil // Schema version is optional
	}

	// Basic semantic version validation (x.y.z format)
	matched, err := regexp.MatchString(`^\d+\.\d+\.\d+$`, version)
	if err != nil {
		return fmt.Errorf("error validating schema version: %w", err)
	}
	if !matched {
		return fmt.Errorf("schema version '%s' must follow semantic versioning format (x.y.z)", version)
	}

	return nil
}

// ValidateEvents validates all events in an EventProduction
func (ep *EventProduction) ValidateEvents() error {
	for i, event := range ep.Events {
		if err := validateEventType(event.Type); err != nil {
			return fmt.Errorf("event %d: %w", i, err)
		}

		if err := validateSchemaVersion(event.SchemaVersion); err != nil {
			return fmt.Errorf("event %d (%s): %w", i, event.Type, err)
		}

		// Validate template expressions in event payload
		for payloadKey, payloadValue := range event.Payload {
			if err := validateTemplateExpression(payloadValue); err != nil {
				return fmt.Errorf("event %d (%s) payload '%s': %w", i, event.Type, payloadKey, err)
			}
		}
	}

	return nil
}

