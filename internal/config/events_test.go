package config

import (
	"testing"
)

func TestValidateEventType(t *testing.T) {
	testCases := []struct {
		name        string
		eventType   string
		expectError bool
	}{
		{"valid snake_case", "library_built", false},
		{"valid single word", "built", false},
		{"valid with numbers", "version_1_released", false},
		{"empty string", "", true},
		{"starts with number", "1_event", true},
		{"contains hyphens", "library-built", true},
		{"contains uppercase", "Library_Built", true},
		{"contains spaces", "library built", true},
		{"contains special chars", "library_built!", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEventType(tc.eventType)
			if tc.expectError && err == nil {
				t.Errorf("expected error for event type %q, got nil", tc.eventType)
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error for event type %q: %v", tc.eventType, err)
			}
		})
	}
}

func TestValidateSchemaVersion(t *testing.T) {
	testCases := []struct {
		name        string
		version     string
		expectError bool
	}{
		{"valid semantic version", "1.2.3", false},
		{"empty string (optional)", "", false},
		{"major version", "2.0.0", false},
		{"patch version", "1.0.1", false},
		{"invalid format missing patch", "1.2", true},
		{"invalid format extra segment", "1.2.3.4", true},
		{"invalid format with text", "v1.2.3", true},
		{"invalid format with hyphens", "1.2.3-alpha", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSchemaVersion(tc.version)
			if tc.expectError && err == nil {
				t.Errorf("expected error for schema version %q, got nil", tc.version)
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error for schema version %q: %v", tc.version, err)
			}
		})
	}
}

func TestEventProduction_ValidateEvents(t *testing.T) {
	testCases := []struct {
		name        string
		events      []Event
		expectError bool
	}{
		{
			name: "valid events",
			events: []Event{
				{Type: "library_built", SchemaVersion: "1.0.0"},
				{Type: "test_completed", SchemaVersion: "2.1.0"},
			},
			expectError: false,
		},
		{
			name: "valid event without schema version",
			events: []Event{
				{Type: "simple_event"},
			},
			expectError: false,
		},
		{
			name: "invalid event type",
			events: []Event{
				{Type: "invalid-type", SchemaVersion: "1.0.0"},
			},
			expectError: true,
		},
		{
			name: "invalid schema version",
			events: []Event{
				{Type: "valid_event", SchemaVersion: "invalid"},
			},
			expectError: true,
		},
		{
			name: "empty event type",
			events: []Event{
				{Type: "", SchemaVersion: "1.0.0"},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ep := &EventProduction{Events: tc.events}
			err := ep.ValidateEvents()
			if tc.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}