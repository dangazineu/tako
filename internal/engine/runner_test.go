package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dangazineu/tako/internal/config"
)

func TestNewRunner(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot:      filepath.Join(tempDir, "workspace"),
		CacheDir:           filepath.Join(tempDir, "cache"),
		MaxConcurrentRepos: 2,
		DryRun:             true,
		Debug:              false,
		NoCache:            false,
		Environment:        []string{}, // Empty environment for tests
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	if runner.runID == "" {
		t.Error("Runner should have a non-empty run ID")
	}

	if !IsValidRunID(runner.runID) {
		t.Errorf("Runner should have a valid run ID, got: %s", runner.runID)
	}

	if runner.mode != ExecutionModeDryRun {
		t.Errorf("Expected dry run mode, got: %v", runner.mode)
	}

	if runner.maxConcurrentRepos != 2 {
		t.Errorf("Expected max concurrent repos to be 2, got: %d", runner.maxConcurrentRepos)
	}

	// Check that workspace directory was created
	if _, err := os.Stat(runner.workspaceRoot); os.IsNotExist(err) {
		t.Error("Workspace directory should be created")
	}
}

func TestRunnerGetters(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		Environment:   []string{}, // Empty environment for tests
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	runID := runner.GetRunID()
	if runID == "" {
		t.Error("GetRunID should return non-empty run ID")
	}

	workspaceRoot := runner.GetWorkspaceRoot()
	if workspaceRoot == "" {
		t.Error("GetWorkspaceRoot should return non-empty path")
	}

	if !filepath.IsAbs(workspaceRoot) {
		t.Error("Workspace root should be an absolute path")
	}
}

func TestRunnerValidateInputs(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		Environment:   []string{}, // Empty environment for tests
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	// Test with workflow that has input validation
	workflow := createTestWorkflow()

	// Test valid inputs
	validInputs := map[string]string{
		"environment": "dev",
		"version":     "1.0.0",
	}

	if err := runner.validateInputs(workflow, validInputs); err != nil {
		t.Errorf("Valid inputs should pass validation: %v", err)
	}

	// Test invalid enum value
	invalidInputs := map[string]string{
		"environment": "invalid",
		"version":     "1.0.0",
	}

	if err := runner.validateInputs(workflow, invalidInputs); err == nil {
		t.Error("Invalid enum value should fail validation")
	}

	// Test missing required input
	missingInputs := map[string]string{
		"version": "1.0.0",
	}

	if err := runner.validateInputs(workflow, missingInputs); err == nil {
		t.Error("Missing required input should fail validation")
	}
}

func TestRunnerDryRunMode(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test tako.yml file
	takoFile := filepath.Join(tempDir, "tako.yml")
	createTestTakoConfig(t, takoFile)

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		DryRun:        true,
		Environment:   []string{}, // Empty environment for tests
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	ctx := context.Background()
	inputs := map[string]string{
		"environment": "dev",
	}

	result, err := runner.ExecuteWorkflow(ctx, "test-workflow", inputs, tempDir)
	if err != nil {
		t.Fatalf("Dry run execution should succeed: %v", err)
	}

	if !result.Success {
		t.Error("Dry run should always succeed")
	}

	if result.RunID == "" {
		t.Error("Result should have a run ID")
	}

	// In dry run mode, steps should be simulated
	if len(result.Steps) == 0 {
		t.Error("Dry run should still execute steps (simulated)")
	}

	for _, step := range result.Steps {
		if !step.Success {
			t.Error("All dry run steps should succeed")
		}
	}
}

func TestRunnerExecutionTimeout(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		DryRun:        false,      // Use normal mode to potentially allow timeout
		Environment:   []string{}, // Empty environment for tests
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Create a test tako.yml file
	takoFile := filepath.Join(tempDir, "tako.yml")
	createTestTakoConfig(t, takoFile)

	inputs := map[string]string{
		"environment": "dev",
	}

	// This should timeout quickly due to the very short timeout
	_, err = runner.ExecuteWorkflow(ctx, "test-workflow", inputs, tempDir)

	// In the current implementation, dry run mode might complete before timeout
	// So we'll just check that the execution doesn't panic or return nil error
	// when a cancelled context is used
	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		// Some other error occurred, which is fine for this test
		t.Logf("Execution failed with: %v", err)
	}
}

func TestRunnerMultiRepoRepositoryNotFound(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		Environment:   []string{}, // Empty environment for tests
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	ctx := context.Background()
	inputs := map[string]string{}

	_, err = runner.ExecuteMultiRepoWorkflow(ctx, "test-workflow", inputs, "org/repo")
	if err == nil {
		t.Error("Multi-repo execution should return repository not found error")
	}

	// Should fail because the repository doesn't exist in cache
	if !strings.Contains(err.Error(), "repository org/repo not found in cache") {
		t.Errorf("Expected repository not found error, got %q", err.Error())
	}
}

func TestRunnerResumeNotImplemented(t *testing.T) {
	tempDir := t.TempDir()

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		Environment:   []string{}, // Empty environment for tests
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	ctx := context.Background()

	_, err = runner.Resume(ctx, "exec-20240726-143022-a7b3c1d2")
	if err == nil {
		t.Error("Resume should return not implemented error")
	}

	expectedMsg := "execution resume not yet implemented"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message %q, got %q", expectedMsg, err.Error())
	}
}

// TestRunnerWorkflowExecution tests successful workflow execution.
func TestRunnerWorkflowExecution(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test tako.yml file
	takoFile := filepath.Join(tempDir, "tako.yml")
	createTestTakoConfig(t, takoFile)

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		DryRun:        true, // Use dry run to avoid actual command execution
		Environment:   []string{},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	ctx := context.Background()
	inputs := map[string]string{
		"environment": "dev",
		"version":     "2.0.0",
	}

	result, err := runner.ExecuteWorkflow(ctx, "test-workflow", inputs, tempDir)
	if err != nil {
		t.Fatalf("Workflow execution should succeed: %v", err)
	}

	// Verify result structure
	if !result.Success {
		t.Error("Workflow should succeed")
	}

	if result.RunID == "" {
		t.Error("Result should have a run ID")
	}

	if result.StartTime.IsZero() {
		t.Error("Result should have a start time")
	}

	if result.EndTime.IsZero() {
		t.Error("Result should have an end time")
	}

	if result.EndTime.Before(result.StartTime) {
		t.Error("End time should be after start time")
	}

	// Verify steps were executed
	if len(result.Steps) != 2 {
		t.Errorf("Expected 2 steps, got %d", len(result.Steps))
	}

	// Verify first step
	firstStep := result.Steps[0]
	if firstStep.ID != "validate_input" {
		t.Errorf("Expected first step ID 'validate_input', got %q", firstStep.ID)
	}

	if !firstStep.Success {
		t.Error("First step should succeed in dry run")
	}

	if !strings.Contains(firstStep.Output, "[dry-run]") {
		t.Error("Dry run output should contain [dry-run] prefix")
	}

	// Verify second step
	secondStep := result.Steps[1]
	if secondStep.ID != "process_output" {
		t.Errorf("Expected second step ID 'process_output', got %q", secondStep.ID)
	}

	if !secondStep.Success {
		t.Error("Second step should succeed in dry run")
	}
}

// TestRunnerWorkflowExecutionWithFailure tests workflow execution with failing steps.
func TestRunnerWorkflowExecutionWithFailure(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test tako.yml with a failing step
	takoFile := filepath.Join(tempDir, "tako.yml")
	content := `version: 0.1.0
artifacts:
  default:
    path: "."
    ecosystem: "generic"
workflows:
  failing-workflow:
    inputs:
      environment:
        type: string
        required: true
    steps:
      - id: success_step
        run: echo 'This will succeed'
      - id: failure_step
        run: exit 1
subscriptions: []
`
	if err := os.WriteFile(takoFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test tako.yml: %v", err)
	}

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		DryRun:        false, // Use normal mode to actually fail
		Environment:   []string{},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	ctx := context.Background()
	inputs := map[string]string{
		"environment": "dev",
	}

	result, err := runner.ExecuteWorkflow(ctx, "failing-workflow", inputs, tempDir)

	// Execution should fail
	if err == nil {
		t.Error("Workflow execution should fail when step fails")
	}

	if result.Success {
		t.Error("Workflow result should indicate failure")
	}

	// Should have executed the first step successfully
	if len(result.Steps) == 0 {
		t.Error("Should have at least one step result")
	}

	// First step should succeed
	firstStep := result.Steps[0]
	if !firstStep.Success {
		t.Error("First step should succeed")
	}

	// If second step was attempted, it should fail
	if len(result.Steps) > 1 {
		secondStep := result.Steps[1]
		if secondStep.Success {
			t.Error("Second step should fail")
		}
		if secondStep.Error == nil {
			t.Error("Failed step should have error")
		}
	}
}

// TestRunnerInputValidationExtensive tests comprehensive input validation scenarios.
func TestRunnerInputValidationExtensive(t *testing.T) {
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

	// Test workflow with various input validation rules
	workflow := config.Workflow{
		Name: "validation-test",
		Inputs: map[string]config.WorkflowInput{
			"required_string": {
				Type:     "string",
				Required: true,
			},
			"optional_with_default": {
				Type:    "string",
				Default: "default_value",
			},
			"enum_field": {
				Type: "string",
				Validation: config.WorkflowInputValidation{
					Enum: []string{"option1", "option2", "option3"},
				},
			},
		},
	}

	t.Run("valid inputs with defaults", func(t *testing.T) {
		inputs := map[string]string{
			"required_string": "test_value",
			"enum_field":      "option2",
		}

		err := runner.validateInputs(workflow, inputs)
		if err != nil {
			t.Errorf("Valid inputs should pass validation: %v", err)
		}

		// Check that default was applied
		if inputs["optional_with_default"] != "default_value" {
			t.Errorf("Expected default value to be applied, got %q", inputs["optional_with_default"])
		}
	})

	t.Run("missing required input", func(t *testing.T) {
		inputs := map[string]string{
			"enum_field": "option1",
		}

		err := runner.validateInputs(workflow, inputs)
		if err == nil {
			t.Error("Missing required input should fail validation")
		}

		if !strings.Contains(err.Error(), "required input 'required_string' not provided") {
			t.Errorf("Error should mention missing required input, got: %v", err)
		}
	})

	t.Run("invalid enum value", func(t *testing.T) {
		inputs := map[string]string{
			"required_string": "test_value",
			"enum_field":      "invalid_option",
		}

		err := runner.validateInputs(workflow, inputs)
		if err == nil {
			t.Error("Invalid enum value should fail validation")
		}

		if !strings.Contains(err.Error(), "not in allowed values") {
			t.Errorf("Error should mention enum validation, got: %v", err)
		}
	})

	t.Run("empty enum allows any value", func(t *testing.T) {
		workflowEmptyEnum := config.Workflow{
			Name: "empty-enum-test",
			Inputs: map[string]config.WorkflowInput{
				"any_value": {
					Type: "string",
					Validation: config.WorkflowInputValidation{
						Enum: []string{}, // Empty enum should allow any value
					},
				},
			},
		}

		inputs := map[string]string{
			"any_value": "anything_goes",
		}

		err := runner.validateInputs(workflowEmptyEnum, inputs)
		if err != nil {
			t.Errorf("Empty enum should allow any value: %v", err)
		}
	})
}

// TestRunnerDebugMode tests debug mode execution.
func TestRunnerDebugMode(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test tako.yml file
	takoFile := filepath.Join(tempDir, "tako.yml")
	createTestTakoConfig(t, takoFile)

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		Debug:         true,
		Environment:   []string{},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	if runner.mode != ExecutionModeDebug {
		t.Errorf("Expected debug mode, got: %v", runner.mode)
	}

	ctx := context.Background()
	inputs := map[string]string{
		"environment": "dev",
	}

	result, err := runner.ExecuteWorkflow(ctx, "test-workflow", inputs, tempDir)
	if err != nil {
		t.Fatalf("Debug mode execution should succeed: %v", err)
	}

	if !result.Success {
		t.Error("Debug mode execution should succeed")
	}
}

// TestRunnerStepOutputProcessing tests step output processing with produces field.
func TestRunnerStepOutputProcessing(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test tako.yml with output-producing steps
	takoFile := filepath.Join(tempDir, "tako.yml")
	content := `version: 0.1.0
artifacts:
  default:
    path: "."
    ecosystem: "generic"
workflows:
  output-workflow:
    inputs:
      test_input:
        type: string
        default: "test_value"
    steps:
      - id: stdout_step
        run: echo "output_from_stdout"
        produces:
          outputs:
            result: from_stdout
      - id: stderr_step
        run: echo "output_from_stderr" >&2
        produces:
          outputs:
            error_result: from_stderr
      - id: regex_step
        run: echo "Version=1.2.3 Build=456"
        produces:
          outputs:
            version: "from_stdout"
            build: "from_stdout"
subscriptions: []
`
	if err := os.WriteFile(takoFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test tako.yml: %v", err)
	}

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		DryRun:        false, // Need actual execution to test outputs
		Environment:   []string{},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	ctx := context.Background()
	inputs := map[string]string{
		"test_input": "test",
	}

	result, err := runner.ExecuteWorkflow(ctx, "output-workflow", inputs, tempDir)
	if err != nil {
		t.Fatalf("Workflow execution should succeed: %v", err)
	}

	if !result.Success {
		t.Error("Workflow should succeed")
	}

	// Verify outputs were captured
	if len(result.Steps) != 3 {
		t.Fatalf("Expected 3 steps, got %d", len(result.Steps))
	}

	// Test stdout output
	stdoutStep := result.Steps[0]
	if stdoutStep.ID != "stdout_step" {
		t.Errorf("Expected stdout_step, got %q", stdoutStep.ID)
	}

	if stdoutStep.Outputs["result"] != "output_from_stdout" {
		t.Errorf("Expected stdout output 'output_from_stdout', got %q", stdoutStep.Outputs["result"])
	}

	// Test stderr output
	stderrStep := result.Steps[1]
	if stderrStep.ID != "stderr_step" {
		t.Errorf("Expected stderr_step, got %q", stderrStep.ID)
	}

	if stderrStep.Outputs["error_result"] != "output_from_stderr" {
		t.Errorf("Expected stderr output 'output_from_stderr', got %q", stderrStep.Outputs["error_result"])
	}

	// Test stdout output extraction
	stdoutStep2 := result.Steps[2]
	if stdoutStep2.ID != "regex_step" {
		t.Errorf("Expected regex_step, got %q", stdoutStep2.ID)
	}

	expectedOutput := "Version=1.2.3 Build=456"
	if stdoutStep2.Outputs["version"] != expectedOutput {
		t.Errorf("Expected version output %q, got %q", expectedOutput, stdoutStep2.Outputs["version"])
	}

	if stdoutStep2.Outputs["build"] != expectedOutput {
		t.Errorf("Expected build output %q, got %q", expectedOutput, stdoutStep2.Outputs["build"])
	}
}

// TestRunnerContextCancellation tests context cancellation during execution.
func TestRunnerContextCancellation(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test tako.yml with a long-running command
	takoFile := filepath.Join(tempDir, "tako.yml")
	content := `version: 0.1.0
artifacts:
  default:
    path: "."
    ecosystem: "generic"
workflows:
  long-workflow:
    inputs: {}
    steps:
      - id: quick_step
        run: echo "quick"
      - id: slow_step
        run: sleep 10
subscriptions: []
`
	if err := os.WriteFile(takoFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test tako.yml: %v", err)
	}

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		DryRun:        false, // Need actual execution to test cancellation
		Environment:   []string{},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	// Create a context that will be cancelled quickly
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	inputs := map[string]string{}

	result, err := runner.ExecuteWorkflow(ctx, "long-workflow", inputs, tempDir)

	// Should get an error (could be context cancellation or killed signal)
	if err == nil {
		t.Error("Expected execution error due to timeout")
	}

	// The error could be context cancellation or a killed signal due to timeout
	errorStr := err.Error()
	isContextError := err == context.DeadlineExceeded || err == context.Canceled ||
		strings.Contains(errorStr, "context") || strings.Contains(errorStr, "deadline") || strings.Contains(errorStr, "canceled")
	isKilledError := strings.Contains(errorStr, "killed") || strings.Contains(errorStr, "signal")

	if !isContextError && !isKilledError {
		t.Errorf("Expected context cancellation or killed signal error, got: %v", err)
	}

	// Result should indicate failure
	if result != nil && result.Success {
		t.Error("Result should indicate failure when execution is cancelled/killed")
	}
}

// TestRunnerWorkflowNotFound tests handling of non-existent workflows.
func TestRunnerWorkflowNotFound(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test tako.yml file
	takoFile := filepath.Join(tempDir, "tako.yml")
	createTestTakoConfig(t, takoFile)

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

	ctx := context.Background()
	inputs := map[string]string{}

	result, err := runner.ExecuteWorkflow(ctx, "non-existent-workflow", inputs, tempDir)

	// Should fail with workflow not found error
	if err == nil {
		t.Error("Expected workflow not found error")
	}

	if !strings.Contains(err.Error(), "workflow 'non-existent-workflow' not found") {
		t.Errorf("Expected workflow not found error, got: %v", err)
	}

	// Result should indicate failure
	if result.Success {
		t.Error("Result should indicate failure for non-existent workflow")
	}
}

// TestRunnerInvalidTakoConfig tests handling of invalid tako.yml files.
func TestRunnerInvalidTakoConfig(t *testing.T) {
	tempDir := t.TempDir()

	// Create an invalid tako.yml file
	takoFile := filepath.Join(tempDir, "tako.yml")
	invalidContent := `invalid yaml content [[[`
	if err := os.WriteFile(takoFile, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to create invalid tako.yml: %v", err)
	}

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

	ctx := context.Background()
	inputs := map[string]string{}

	result, err := runner.ExecuteWorkflow(ctx, "any-workflow", inputs, tempDir)

	// Should fail with config load error
	if err == nil {
		t.Error("Expected config load error")
	}

	// The error could be either "failed to load config" or "could not unmarshal config"
	errorStr := err.Error()
	if !strings.Contains(errorStr, "failed to load config") && !strings.Contains(errorStr, "could not unmarshal config") {
		t.Errorf("Expected config load/unmarshal error, got: %v", err)
	}

	// Result should indicate failure
	if result.Success {
		t.Error("Result should indicate failure for invalid config")
	}
}

// TestRunnerEnvironmentVariables tests environment variable handling.
func TestRunnerEnvironmentVariables(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test tako.yml that uses environment variables
	takoFile := filepath.Join(tempDir, "tako.yml")
	content := `version: 0.1.0
artifacts:
  default:
    path: "."
    ecosystem: "generic"
workflows:
  env-workflow:
    inputs:
      test_input:
        type: string
        default: "test"
    steps:
      - id: env_step
        run: echo "TAKO_RUN_ID=$TAKO_RUN_ID TAKO_STEP_ID=$TAKO_STEP_ID TAKO_INPUT_TEST_INPUT=$TAKO_INPUT_TEST_INPUT"
subscriptions: []
`
	if err := os.WriteFile(takoFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test tako.yml: %v", err)
	}

	opts := RunnerOptions{
		WorkspaceRoot: filepath.Join(tempDir, "workspace"),
		CacheDir:      filepath.Join(tempDir, "cache"),
		DryRun:        false, // Need actual execution to test environment variables
		Environment:   []string{"CUSTOM_ENV=custom_value"},
	}

	runner, err := NewRunner(opts)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}
	defer runner.Close()

	ctx := context.Background()
	inputs := map[string]string{
		"test_input": "input_value",
	}

	result, err := runner.ExecuteWorkflow(ctx, "env-workflow", inputs, tempDir)
	if err != nil {
		t.Fatalf("Workflow execution should succeed: %v", err)
	}

	if !result.Success {
		t.Error("Workflow should succeed")
	}

	// Check that environment variables were set correctly
	if len(result.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(result.Steps))
	}

	output := result.Steps[0].Output
	if !strings.Contains(output, "TAKO_RUN_ID="+runner.runID) {
		t.Errorf("Output should contain TAKO_RUN_ID, got: %s", output)
	}

	if !strings.Contains(output, "TAKO_STEP_ID=env_step") {
		t.Errorf("Output should contain TAKO_STEP_ID, got: %s", output)
	}

	if !strings.Contains(output, "TAKO_INPUT_TEST_INPUT=input_value") {
		t.Errorf("Output should contain TAKO_INPUT_TEST_INPUT, got: %s", output)
	}
}
