package engine

import (
	"strings"
	"testing"
	"time"
)

func TestTemplateEngine_BasicExpansion(t *testing.T) {
	engine := NewTemplateEngine()

	context := &TemplateContext{
		Inputs: map[string]string{
			"version":     "1.2.3",
			"environment": "prod",
		},
		Steps: map[string]map[string]string{
			"build": {
				"artifact": "app-1.2.3.jar",
				"status":   "success",
			},
		},
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "simple input substitution",
			template: "Deploying version {{ .Inputs.version }}",
			expected: "Deploying version 1.2.3",
		},
		{
			name:     "step output substitution",
			template: "Built artifact: {{ .Steps.build.artifact }}",
			expected: "Built artifact: app-1.2.3.jar",
		},
		{
			name:     "multiple substitutions",
			template: "Deploying {{ .Inputs.version }} to {{ .Inputs.environment }} - artifact: {{ .Steps.build.artifact }}",
			expected: "Deploying 1.2.3 to prod - artifact: app-1.2.3.jar",
		},
		{
			name:     "empty template",
			template: "",
			expected: "",
		},
		{
			name:     "no substitution",
			template: "Hello World",
			expected: "Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.ExpandTemplate(tt.template, context)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTemplateEngine_EventContext(t *testing.T) {
	engine := NewTemplateEngine()

	context := &TemplateContext{
		Event: &EventContext{
			Type:   "artifact_updated",
			Source: "github.com/user/repo",
			Payload: map[string]interface{}{
				"version": "2.0.0",
				"artifact": map[string]interface{}{
					"name": "myapp",
					"type": "jar",
				},
				"dependencies": []interface{}{
					map[string]interface{}{
						"name":    "lib1",
						"version": "1.0.0",
					},
					map[string]interface{}{
						"name":    "lib2",
						"version": "2.1.0",
					},
				},
			},
			Timestamp: time.Now(),
		},
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "event type",
			template: "Event: {{ .Event.Type }}",
			expected: "Event: artifact_updated",
		},
		{
			name:     "event payload field",
			template: "Version: {{ .Event.Payload.version }}",
			expected: "Version: 2.0.0",
		},
		{
			name:     "nested payload field",
			template: "Artifact: {{ .Event.Payload.artifact.name }}",
			expected: "Artifact: myapp",
		},
		{
			name:     "event source",
			template: "Source: {{ .Event.Source }}",
			expected: "Source: github.com/user/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.ExpandTemplate(tt.template, context)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTemplateEngine_SecurityFunctions(t *testing.T) {
	engine := NewTemplateEngine()

	context := &TemplateContext{
		Inputs: map[string]string{
			"unsafe_input": "test'; rm -rf /; echo 'hacked",
			"json_data":    "test\"with\\quotes\nand\tspecial\rchars",
			"url_param":    "hello world & special chars",
			"html_content": "<script>alert('xss')</script>",
		},
	}

	tests := []struct {
		name     string
		template string
		contains string
	}{
		{
			name:     "shell quote safety",
			template: "{{ .Inputs.unsafe_input | shell_quote }}",
			contains: "'test'", // Should be safely quoted
		},
		{
			name:     "json escape",
			template: "{{ .Inputs.json_data | json_escape }}",
			contains: "test\\\"with\\\\quotes\\nand\\tspecial\\rchars",
		},
		{
			name:     "url encode",
			template: "{{ .Inputs.url_param | url_encode }}",
			contains: "hello+world",
		},
		{
			name:     "html escape",
			template: "{{ .Inputs.html_content | html_escape }}",
			contains: "&lt;script&gt;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.ExpandTemplate(tt.template, context)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}
}

func TestTemplateEngine_UtilityFunctions(t *testing.T) {
	engine := NewTemplateEngine()

	context := &TemplateContext{
		Inputs: map[string]string{
			"empty_val":  "",
			"text":       "  Hello World  ",
			"number":     "42",
			"bool_true":  "true",
			"bool_false": "false",
		},
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "default value for empty",
			template: "{{ .Inputs.empty_val | default \"fallback\" }}",
			expected: "fallback",
		},
		{
			name:     "default value for non-empty",
			template: "{{ .Inputs.text | default \"fallback\" }}",
			expected: "  Hello World  ",
		},
		{
			name:     "trim whitespace",
			template: "{{ .Inputs.text | trim }}",
			expected: "Hello World",
		},
		{
			name:     "uppercase",
			template: "{{ .Inputs.text | upper }}",
			expected: "  HELLO WORLD  ",
		},
		{
			name:     "lowercase",
			template: "{{ .Inputs.text | lower }}",
			expected: "  hello world  ",
		},
		{
			name:     "string conversion",
			template: "{{ .Inputs.number | to_string }}",
			expected: "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.ExpandTemplate(tt.template, context)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTemplateEngine_ConditionalFunctions(t *testing.T) {
	engine := NewTemplateEngine()

	context := &TemplateContext{
		Inputs: map[string]string{
			"env":     "prod",
			"enabled": "true",
			"count":   "5",
		},
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "if-then-else true condition",
			template: "{{ if_then_else (eq .Inputs.env \"prod\") \"production\" \"development\" }}",
			expected: "production",
		},
		{
			name:     "if-then-else false condition",
			template: "{{ if_then_else (eq .Inputs.env \"dev\") \"production\" \"development\" }}",
			expected: "development",
		},
		{
			name:     "logical and true",
			template: "{{ and .Inputs.enabled (eq .Inputs.count \"5\") }}",
			expected: "true",
		},
		{
			name:     "logical or",
			template: "{{ or (eq .Inputs.env \"dev\") (eq .Inputs.env \"prod\") }}",
			expected: "true",
		},
		{
			name:     "logical not",
			template: "{{ not (eq .Inputs.env \"dev\") }}",
			expected: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.ExpandTemplate(tt.template, context)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTemplateEngine_EventProcessingFunctions(t *testing.T) {
	engine := NewTemplateEngine()

	context := &TemplateContext{
		Event: &EventContext{
			Type:   "deployment_complete",
			Source: "ci/cd",
			Payload: map[string]interface{}{
				"version": "1.0.0",
				"metadata": map[string]interface{}{
					"commit": "abc123",
					"branch": "main",
				},
				"services": []interface{}{
					map[string]interface{}{
						"name": "api",
						"port": 8080,
					},
					map[string]interface{}{
						"name": "web",
						"port": 3000,
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		template string
		contains string
	}{
		{
			name:     "event field extraction",
			template: "{{ event_field \"version\" .Event }}",
			contains: "1.0.0",
		},
		{
			name:     "nested event field",
			template: "{{ event_field \"metadata.commit\" .Event }}",
			contains: "abc123",
		},
		{
			name:     "event has field true",
			template: "{{ event_has_field \"version\" .Event }}",
			contains: "true",
		},
		{
			name:     "event has field false",
			template: "{{ event_has_field \"nonexistent\" .Event }}",
			contains: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.ExpandTemplate(tt.template, context)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}
}

func TestTemplateEngine_Caching(t *testing.T) {
	engine := NewTemplateEngine()

	context := &TemplateContext{
		Inputs: map[string]string{
			"value": "test",
		},
	}

	template := "Hello {{ .Inputs.value }}"

	// First execution - should parse and cache
	result1, err := engine.ExpandTemplate(template, context)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Second execution - should use cache
	result2, err := engine.ExpandTemplate(template, context)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result1 != result2 {
		t.Errorf("Cached result should be identical: %q vs %q", result1, result2)
	}

	// Check cache stats
	stats := engine.GetCacheStats()
	if stats["entries"].(int) == 0 {
		t.Error("Cache should contain at least one entry")
	}
}

func TestTemplateEngine_ValidationErrors(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name     string
		template string
		hasError bool
	}{
		{
			name:     "valid template",
			template: "Hello {{ .Inputs.name }}",
			hasError: false,
		},
		{
			name:     "invalid template syntax",
			template: "Hello {{ .Inputs.name",
			hasError: true,
		},
		{
			name:     "invalid function call",
			template: "{{ unknown_function .Inputs.name }}",
			hasError: true,
		},
		{
			name:     "empty template",
			template: "",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.ValidateTemplate(tt.template)
			if tt.hasError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestTemplateEngine_ComplexWorkflow(t *testing.T) {
	engine := NewTemplateEngine()

	context := &TemplateContext{
		Inputs: map[string]string{
			"environment": "prod",
			"version":     "2.1.0",
			"service":     "api",
		},
		Steps: map[string]map[string]string{
			"build": {
				"artifact": "api-2.1.0.jar",
				"status":   "success",
			},
			"test": {
				"coverage": "95",
				"status":   "passed",
			},
		},
		Event: &EventContext{
			Type:   "deployment_requested",
			Source: "github.com/org/repo",
			Payload: map[string]interface{}{
				"deployment": map[string]interface{}{
					"target":   "prod-cluster",
					"replicas": 3,
					"strategy": "rolling",
				},
				"metadata": map[string]interface{}{
					"commit":    "abc123def",
					"author":    "dev@company.com",
					"timestamp": "2024-01-15T10:30:00Z",
				},
			},
		},
	}

	template := `#!/bin/bash
set -e

# Deployment script for {{ .Inputs.service }}
SERVICE={{ .Inputs.service | shell_quote }}
VERSION={{ .Inputs.version | shell_quote }}
ENVIRONMENT={{ .Inputs.environment | shell_quote }}
ARTIFACT={{ .Steps.build.artifact | shell_quote }}

# Event-driven deployment details
TARGET={{ .Event.Payload.deployment.target | shell_quote }}
REPLICAS={{ .Event.Payload.deployment.replicas }}
STRATEGY={{ .Event.Payload.deployment.strategy | shell_quote }}

echo "Deploying $SERVICE version $VERSION to $ENVIRONMENT"
echo "Target: $TARGET with $REPLICAS replicas using $STRATEGY strategy"

{{- if eq .Inputs.environment "prod" }}
echo "Production deployment - enabling additional checks"
{{- end }}

# Validate build artifacts
if [[ "{{ .Steps.build.status }}" != "success" ]]; then
    echo "Build failed, aborting deployment"
    exit 1
fi

if [[ {{ .Steps.test.coverage }} -lt 90 ]]; then
    echo "Test coverage below threshold, aborting deployment"
    exit 1
fi

echo "Deployment completed successfully"`

	result, err := engine.ExpandTemplate(template, context)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify key components are properly substituted
	expectedContains := []string{
		"SERVICE=api",
		"VERSION=2.1.0",
		"ENVIRONMENT=prod",
		"ARTIFACT=api-2.1.0.jar",
		"TARGET=prod-cluster",
		"REPLICAS=3",
		"STRATEGY=rolling",
		"Production deployment - enabling additional checks",
		"Deployment completed successfully",
	}

	for _, expected := range expectedContains {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain %q", expected)
		}
	}
}
