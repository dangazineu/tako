package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dangazineu/tako/internal/config"
)

func TestRunner_expandTemplate(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		Environment:   []string{},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	// Test basic input expansion
	inputs := map[string]string{
		"environment": "dev",
		"version":     "1.2.3",
	}

	stepOutputs := map[string]map[string]string{
		"build": {
			"artifact": "app-1.2.3.jar",
			"status":   "success",
		},
		"test": {
			"result":   "passed",
			"coverage": "85%",
		},
	}

	tests := []struct {
		name        string
		template    string
		expected    string
		shouldError bool
	}{
		{
			name:     "simple input substitution",
			template: "Deploying version {{ .inputs.version }} to {{ .inputs.environment }}",
			expected: "Deploying version 1.2.3 to dev",
		},
		{
			name:     "step output substitution",
			template: "Built artifact: {{ .steps.build.artifact }}",
			expected: "Built artifact: app-1.2.3.jar",
		},
		{
			name:     "complex template with multiple substitutions",
			template: "Version {{ .inputs.version }} built as {{ .steps.build.artifact }} with status {{ .steps.build.status }} and test result {{ .steps.test.result }}",
			expected: "Version 1.2.3 built as app-1.2.3.jar with status success and test result passed",
		},
		{
			name:     "no substitution needed",
			template: "echo 'Hello World'",
			expected: "echo 'Hello World'",
		},
		{
			name:     "empty template",
			template: "",
			expected: "",
		},
		{
			name:        "invalid template syntax",
			template:    "{{ .inputs.invalid }}",
			shouldError: false, // Go templates handle missing keys gracefully
			expected:    "<no value>",
		},
		{
			name:        "malformed template",
			template:    "{{ .inputs.version",
			shouldError: true,
		},
		{
			name:     "conditional logic",
			template: "{{ if eq .inputs.environment \"dev\" }}Development{{ else }}Production{{ end }}",
			expected: "Development",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runner.expandTemplate(tt.template, inputs, stepOutputs)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error for template %q, but got none", tt.template)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for template %q: %v", tt.template, err)
				return
			}

			if result != tt.expected {
				t.Errorf("Template %q: expected %q, got %q", tt.template, tt.expected, result)
			}
		})
	}
}

func TestRunner_expandTemplate_EdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		Environment:   []string{},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	// Test with nil/empty inputs
	result, err := runner.expandTemplate("{{ .inputs.nonexistent }}", nil, nil)
	if err != nil {
		t.Errorf("Should handle nil inputs gracefully: %v", err)
	}
	if result != "<no value>" {
		t.Errorf("Expected '<no value>' for missing input, got %q", result)
	}

	// Test with empty maps
	emptyInputs := make(map[string]string)
	emptyStepOutputs := make(map[string]map[string]string)

	_, err = runner.expandTemplate("{{ .inputs.test }}", emptyInputs, emptyStepOutputs)
	if err != nil {
		t.Errorf("Should handle empty maps gracefully: %v", err)
	}

	// Test with special characters
	inputs := map[string]string{
		"special": "value with spaces & symbols!",
	}

	result, err = runner.expandTemplate("{{ .inputs.special }}", inputs, emptyStepOutputs)
	if err != nil {
		t.Errorf("Should handle special characters: %v", err)
	}
	if result != "value with spaces & symbols!" {
		t.Errorf("Expected special characters to be preserved, got %q", result)
	}
}

func TestRunner_executeBuiltinStep(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		Environment:   []string{},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	// Create a test step with uses field
	step := config.WorkflowStep{
		ID:   "test-builtin",
		Uses: "tako/fan-out@v1",
	}

	stepID := "test-builtin"
	runner.state.StartExecution("test", "/tmp", map[string]string{})
	startTime := time.Now()

	// Execute built-in step (should return not implemented error)
	result, err := runner.executeBuiltinStep(step, stepID, startTime)

	// Should return error indicating not implemented
	if err == nil {
		t.Error("Expected error for unimplemented built-in step")
	}

	expectedErrMsg := "built-in steps not yet implemented: tako/fan-out@v1"
	if err.Error() != expectedErrMsg {
		t.Errorf("Expected error message %q, got %q", expectedErrMsg, err.Error())
	}

	// Check result properties
	if result.ID != stepID {
		t.Errorf("Expected step ID %s, got %s", stepID, result.ID)
	}

	if result.Success {
		t.Error("Expected step to fail")
	}

	if result.Error == nil {
		t.Error("Expected error in step result")
	}

	if result.Error.Error() != expectedErrMsg {
		t.Errorf("Expected error in result: %q, got %q", expectedErrMsg, result.Error.Error())
	}
}

func TestRunner_executeBuiltinStep_DifferentBuiltins(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		Environment:   []string{},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	// Test different built-in step types
	builtinSteps := []string{
		"tako/fan-out@v1",
		"tako/aggregate@v1",
		"tako/notify@v1",
		"actions/checkout@v3",
		"custom/action@latest",
	}

	runner.state.StartExecution("test", "/tmp", map[string]string{})

	for _, builtin := range builtinSteps {
		t.Run(builtin, func(t *testing.T) {
			step := config.WorkflowStep{
				ID:   "test-" + builtin,
				Uses: builtin,
			}

			startTime := time.Now()
			result, err := runner.executeBuiltinStep(step, step.ID, startTime)

			// All should return not implemented error
			if err == nil {
				t.Error("Expected error for unimplemented built-in step")
			}

			expectedErrMsg := "built-in steps not yet implemented: " + builtin
			if err.Error() != expectedErrMsg {
				t.Errorf("Expected error message %q, got %q", expectedErrMsg, err.Error())
			}

			if result.Success {
				t.Error("Expected step to fail")
			}
		})
	}
}

func TestRunner_validateInputValue_Comprehensive(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		Environment:   []string{},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	tests := []struct {
		name        string
		input       config.WorkflowInput
		value       string
		shouldError bool
	}{
		{
			name: "valid enum value",
			input: config.WorkflowInput{
				Validation: config.WorkflowInputValidation{
					Enum: []string{"dev", "staging", "prod"},
				},
			},
			value:       "dev",
			shouldError: false,
		},
		{
			name: "invalid enum value",
			input: config.WorkflowInput{
				Validation: config.WorkflowInputValidation{
					Enum: []string{"dev", "staging", "prod"},
				},
			},
			value:       "invalid",
			shouldError: true,
		},
		{
			name: "no validation rules",
			input: config.WorkflowInput{
				Type: "string",
			},
			value:       "any value",
			shouldError: false,
		},
		{
			name: "empty enum list",
			input: config.WorkflowInput{
				Validation: config.WorkflowInputValidation{
					Enum: []string{},
				},
			},
			value:       "any value",
			shouldError: false,
		},
		{
			name: "case sensitive enum",
			input: config.WorkflowInput{
				Validation: config.WorkflowInputValidation{
					Enum: []string{"Dev", "Staging", "Prod"},
				},
			},
			value:       "dev",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runner.validateInputValue("test-input", tt.input, tt.value)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected validation error")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected validation error: %v", err)
				}
			}
		})
	}
}

func TestRunner_executeStep_BuiltinVsShell(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		Environment:   []string{},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	runner.state.StartExecution("test", tempDir, map[string]string{})

	ctx := context.Background()
	inputs := map[string]string{"test": "value"}
	stepOutputs := make(map[string]map[string]string)

	// Test built-in step (has 'uses' field)
	builtinStep := config.WorkflowStep{
		ID:   "builtin-test",
		Uses: "tako/fan-out@v1",
	}

	result, err := runner.executeStep(ctx, builtinStep, tempDir, inputs, stepOutputs)
	if err == nil {
		t.Error("Expected error for built-in step")
	}
	if result.Success {
		t.Error("Built-in step should fail")
	}

	// Test shell step (has 'run' field)
	shellStep := config.WorkflowStep{
		ID:  "shell-test",
		Run: "echo 'test shell step'",
	}

	// Test in dry-run mode to avoid actual execution
	runner.mode = ExecutionModeDryRun
	result, err = runner.executeStep(ctx, shellStep, tempDir, inputs, stepOutputs)
	if err != nil {
		t.Errorf("Shell step should succeed in dry-run: %v", err)
	}
	if !result.Success {
		t.Error("Shell step should succeed in dry-run")
	}
}
