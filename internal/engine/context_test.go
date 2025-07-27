package engine

import (
	"testing"
	"time"
)

func TestContextBuilder(t *testing.T) {
	builder := NewContextBuilder()
	
	inputs := map[string]string{
		"env":     "prod",
		"version": "1.0.0",
	}
	
	stepOutputs := map[string]map[string]string{
		"build": {
			"artifact": "app-1.0.0.jar",
			"status":   "success",
		},
	}
	
	payload := map[string]interface{}{
		"commit": "abc123",
		"author": "dev@company.com",
	}
	
	artifacts := []ArtifactInfo{
		{
			Name:    "lib1",
			Version: "2.0.0",
			Source:  "registry.example.com",
		},
	}
	
	context := builder.
		WithInputs(inputs).
		WithStepOutputs(stepOutputs).
		WithEvent("deployment_started", "ci/cd", payload).
		WithEventVersion("1.0").
		WithLegacyTrigger(artifacts).
		Build()
	
	// Verify inputs
	if context.Inputs["env"] != "prod" {
		t.Errorf("Expected env=prod, got %s", context.Inputs["env"])
	}
	
	// Verify step outputs
	if context.Steps["build"]["artifact"] != "app-1.0.0.jar" {
		t.Errorf("Expected artifact=app-1.0.0.jar, got %s", context.Steps["build"]["artifact"])
	}
	
	// Verify event context
	if context.Event == nil {
		t.Fatal("Event context should not be nil")
	}
	if context.Event.Type != "deployment_started" {
		t.Errorf("Expected event type=deployment_started, got %s", context.Event.Type)
	}
	if context.Event.Version != "1.0" {
		t.Errorf("Expected event version=1.0, got %s", context.Event.Version)
	}
	if context.Event.Payload["commit"] != "abc123" {
		t.Errorf("Expected commit=abc123, got %v", context.Event.Payload["commit"])
	}
	
	// Verify legacy trigger context
	if context.Trigger == nil {
		t.Fatal("Trigger context should not be nil")
	}
	if len(context.Trigger.Artifacts) != 1 {
		t.Errorf("Expected 1 artifact, got %d", len(context.Trigger.Artifacts))
	}
	if context.Trigger.Artifacts[0].Name != "lib1" {
		t.Errorf("Expected artifact name=lib1, got %s", context.Trigger.Artifacts[0].Name)
	}
}

func TestEventFieldExtraction(t *testing.T) {
	event := &EventContext{
		Type:   "test_event",
		Source: "test_source",
		Payload: map[string]interface{}{
			"version": "1.0.0",
			"metadata": map[string]interface{}{
				"commit": "abc123",
				"author": "dev@company.com",
				"nested": map[string]interface{}{
					"deep": "value",
				},
			},
			"tags": []interface{}{"tag1", "tag2"},
		},
	}
	
	tests := []struct {
		name     string
		field    string
		expected interface{}
	}{
		{
			name:     "simple field",
			field:    "version",
			expected: "1.0.0",
		},
		{
			name:     "nested field",
			field:    "metadata.commit",
			expected: "abc123",
		},
		{
			name:     "deeply nested field",
			field:    "metadata.nested.deep",
			expected: "value",
		},
		{
			name:     "non-existent field",
			field:    "nonexistent",
			expected: nil,
		},
		{
			name:     "empty field returns whole payload",
			field:    "",
			expected: "WHOLE_PAYLOAD", // Special marker for map comparison
		},
		{
			name:     "invalid nested path",
			field:    "version.invalid",
			expected: nil,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eventField(tt.field, event)
			
			// Special handling for whole payload comparison
			if tt.expected == "WHOLE_PAYLOAD" {
				if payload, ok := result.(map[string]interface{}); !ok {
					t.Errorf("Expected map[string]interface{}, got %T", result)
				} else if payload["version"] != "1.0.0" {
					t.Errorf("Expected whole payload with version=1.0.0, got %v", payload)
				}
				return
			}
			
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEventHasField(t *testing.T) {
	event := &EventContext{
		Type:   "test_event",
		Source: "test_source",
		Payload: map[string]interface{}{
			"version": "1.0.0",
			"metadata": map[string]interface{}{
				"commit": "abc123",
				"nested": map[string]interface{}{
					"deep": "value",
				},
			},
		},
	}
	
	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{
			name:     "existing field",
			field:    "version",
			expected: true,
		},
		{
			name:     "existing nested field",
			field:    "metadata.commit",
			expected: true,
		},
		{
			name:     "existing deeply nested field",
			field:    "metadata.nested.deep",
			expected: true,
		},
		{
			name:     "non-existent field",
			field:    "nonexistent",
			expected: false,
		},
		{
			name:     "invalid nested path",
			field:    "version.invalid",
			expected: false,
		},
		{
			name:     "empty field",
			field:    "",
			expected: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eventHasField(tt.field, event)
			if result != tt.expected {
				t.Errorf("Expected %t, got %t", tt.expected, result)
			}
		})
	}
}

func TestEventFilter(t *testing.T) {
	event := &EventContext{
		Type:   "test_event",
		Source: "test_source",
		Payload: map[string]interface{}{
			"version":      "1.0.0",
			"version_tag":  "v1.0.0",
			"build_number": "123",
			"test_results": "passed",
			"metadata":     map[string]interface{}{"key": "value"},
		},
	}
	
	result := eventFilter("version", event)
	
	// Should include keys that contain "version"
	if _, exists := result["version"]; !exists {
		t.Error("Expected 'version' key in filtered result")
	}
	if _, exists := result["version_tag"]; !exists {
		t.Error("Expected 'version_tag' key in filtered result")
	}
	
	// Should not include keys that don't contain "version"
	if _, exists := result["build_number"]; exists {
		t.Error("Did not expect 'build_number' key in filtered result")
	}
	if _, exists := result["test_results"]; exists {
		t.Error("Did not expect 'test_results' key in filtered result")
	}
}

func TestValidateContext(t *testing.T) {
	t.Run("valid context", func(t *testing.T) {
		context := &TemplateContext{
			Inputs: map[string]string{"key": "value"},
			Steps:  map[string]map[string]string{"step1": {"output": "value"}},
			Event: &EventContext{
				Type:      "test",
				Source:    "source",
				Payload:   map[string]interface{}{"key": "value"},
				Timestamp: time.Now(),
			},
			Trigger: &TriggerContext{
				Artifacts: []ArtifactInfo{
					{Name: "artifact", Version: "1.0.0", Source: "source"},
				},
			},
		}
		
		err := ValidateContext(context)
		if err != nil {
			t.Errorf("Expected valid context, got error: %v", err)
		}
	})
	
	t.Run("nil context", func(t *testing.T) {
		err := ValidateContext(nil)
		if err == nil {
			t.Error("Expected error for nil context")
		}
	})
	
	t.Run("context with nil inputs", func(t *testing.T) {
		context := &TemplateContext{
			Inputs: nil,
			Steps:  map[string]map[string]string{},
		}
		
		err := ValidateContext(context)
		if err != nil {
			t.Errorf("Should handle nil inputs gracefully, got error: %v", err)
		}
		
		// Should initialize inputs
		if context.Inputs == nil {
			t.Error("Inputs should be initialized")
		}
	})
	
	t.Run("invalid event context", func(t *testing.T) {
		context := &TemplateContext{
			Inputs: map[string]string{},
			Steps:  map[string]map[string]string{},
			Event: &EventContext{
				Type:   "", // Invalid: empty type
				Source: "source",
			},
		}
		
		err := ValidateContext(context)
		if err == nil {
			t.Error("Expected error for invalid event context")
		}
	})
	
	t.Run("invalid trigger context", func(t *testing.T) {
		context := &TemplateContext{
			Inputs: map[string]string{},
			Steps:  map[string]map[string]string{},
			Trigger: &TriggerContext{
				Artifacts: []ArtifactInfo{
					{Name: "", Version: "1.0.0"}, // Invalid: empty name
				},
			},
		}
		
		err := ValidateContext(context)
		if err == nil {
			t.Error("Expected error for invalid trigger context")
		}
	})
}

func TestMergeContexts(t *testing.T) {
	context1 := &TemplateContext{
		Inputs: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		Steps: map[string]map[string]string{
			"step1": {"output1": "value1"},
		},
		Event: &EventContext{
			Type:   "event1",
			Source: "source1",
		},
	}
	
	context2 := &TemplateContext{
		Inputs: map[string]string{
			"key2": "newvalue2", // Override
			"key3": "value3",    // New
		},
		Steps: map[string]map[string]string{
			"step1": {"output2": "value2"}, // Additional output for same step
			"step2": {"output1": "value1"}, // New step
		},
		Event: &EventContext{
			Type:   "event2", // Override
			Source: "source2",
		},
	}
	
	merged := MergeContexts(context1, context2)
	
	// Check merged inputs
	if merged.Inputs["key1"] != "value1" {
		t.Errorf("Expected key1=value1, got %s", merged.Inputs["key1"])
	}
	if merged.Inputs["key2"] != "newvalue2" {
		t.Errorf("Expected key2=newvalue2 (overridden), got %s", merged.Inputs["key2"])
	}
	if merged.Inputs["key3"] != "value3" {
		t.Errorf("Expected key3=value3, got %s", merged.Inputs["key3"])
	}
	
	// Check merged steps
	if merged.Steps["step1"]["output1"] != "value1" {
		t.Errorf("Expected step1.output1=value1, got %s", merged.Steps["step1"]["output1"])
	}
	if merged.Steps["step1"]["output2"] != "value2" {
		t.Errorf("Expected step1.output2=value2, got %s", merged.Steps["step1"]["output2"])
	}
	if merged.Steps["step2"]["output1"] != "value1" {
		t.Errorf("Expected step2.output1=value1, got %s", merged.Steps["step2"]["output1"])
	}
	
	// Check event override
	if merged.Event.Type != "event2" {
		t.Errorf("Expected event type=event2 (overridden), got %s", merged.Event.Type)
	}
}

func TestCloneContext(t *testing.T) {
	original := &TemplateContext{
		Inputs: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		Steps: map[string]map[string]string{
			"step1": {"output1": "value1"},
		},
		Event: &EventContext{
			Type:   "test_event",
			Source: "test_source",
			Payload: map[string]interface{}{
				"version": "1.0.0",
				"metadata": map[string]interface{}{
					"commit": "abc123",
				},
			},
			Timestamp: time.Now(),
		},
		Trigger: &TriggerContext{
			Artifacts: []ArtifactInfo{
				{Name: "artifact", Version: "1.0.0", Source: "source"},
			},
		},
	}
	
	cloned := CloneContext(original)
	
	// Verify deep copy - changes to clone shouldn't affect original
	cloned.Inputs["key1"] = "modified"
	cloned.Steps["step1"]["output1"] = "modified"
	cloned.Event.Type = "modified"
	cloned.Event.Payload["version"] = "modified"
	cloned.Trigger.Artifacts[0].Name = "modified"
	
	// Original should be unchanged
	if original.Inputs["key1"] != "value1" {
		t.Error("Original inputs were modified")
	}
	if original.Steps["step1"]["output1"] != "value1" {
		t.Error("Original step outputs were modified")
	}
	if original.Event.Type != "test_event" {
		t.Error("Original event type was modified")
	}
	if original.Event.Payload["version"] != "1.0.0" {
		t.Error("Original event payload was modified")
	}
	if original.Trigger.Artifacts[0].Name != "artifact" {
		t.Error("Original trigger artifacts were modified")
	}
}

func TestCloneContextNil(t *testing.T) {
	cloned := CloneContext(nil)
	if cloned != nil {
		t.Error("Cloning nil context should return nil")
	}
}

func TestEventContextValidation(t *testing.T) {
	tests := []struct {
		name    string
		event   *EventContext
		wantErr bool
	}{
		{
			name: "valid event",
			event: &EventContext{
				Type:      "test",
				Source:    "source",
				Payload:   map[string]interface{}{"key": "value"},
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "empty type",
			event: &EventContext{
				Type:      "",
				Source:    "source",
				Timestamp: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "empty source",
			event: &EventContext{
				Type:      "test",
				Source:    "",
				Timestamp: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "zero timestamp",
			event: &EventContext{
				Type:      "test",
				Source:    "source",
				Timestamp: time.Time{},
			},
			wantErr: true,
		},
		{
			name: "nil payload gets initialized",
			event: &EventContext{
				Type:      "test",
				Source:    "source",
				Payload:   nil,
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEventContext(tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEventContext() error = %v, wantErr %v", err, tt.wantErr)
			}
			
			// Check payload initialization
			if !tt.wantErr && tt.event.Payload == nil {
				t.Error("Payload should be initialized for valid events")
			}
		})
	}
}

func TestTriggerContextValidation(t *testing.T) {
	tests := []struct {
		name    string
		trigger *TriggerContext
		wantErr bool
	}{
		{
			name: "valid trigger",
			trigger: &TriggerContext{
				Artifacts: []ArtifactInfo{
					{Name: "artifact", Version: "1.0.0", Source: "source"},
				},
			},
			wantErr: false,
		},
		{
			name: "nil artifacts gets initialized",
			trigger: &TriggerContext{
				Artifacts: nil,
			},
			wantErr: false,
		},
		{
			name: "empty artifact name",
			trigger: &TriggerContext{
				Artifacts: []ArtifactInfo{
					{Name: "", Version: "1.0.0"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty artifact version",
			trigger: &TriggerContext{
				Artifacts: []ArtifactInfo{
					{Name: "artifact", Version: ""},
				},
			},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTriggerContext(tt.trigger)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTriggerContext() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}