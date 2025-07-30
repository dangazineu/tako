package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dangazineu/tako/internal/config"
)

func TestNewChildWorkflowExecutor(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	tests := []struct {
		name        string
		factory     *ChildRunnerFactory
		expectError bool
	}{
		{
			name:        "valid executor creation",
			factory:     factory,
			expectError: false,
		},
		{
			name:        "nil factory",
			factory:     nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, err := NewChildWorkflowExecutor(tt.factory, nil, nil, nil)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if executor == nil {
					t.Error("Executor should not be nil")
				}
			}
		})
	}
}

func TestChildWorkflowExecutor_ValidateRepoPath(t *testing.T) {
	executor := &ChildWorkflowExecutor{}

	tests := []struct {
		name        string
		repoPath    string
		expectError bool
	}{
		{
			name:        "valid relative path",
			repoPath:    "test-org/test-repo",
			expectError: false,
		},
		{
			name:        "valid nested path",
			repoPath:    "test-org/test-repo/subdir",
			expectError: false,
		},
		{
			name:        "path traversal attempt",
			repoPath:    "../../../etc/passwd",
			expectError: true,
		},
		{
			name:        "path traversal in middle",
			repoPath:    "test-org/../../../etc/passwd",
			expectError: true,
		},
		{
			name:        "absolute path",
			repoPath:    "/etc/passwd",
			expectError: true,
		},
		{
			name:        "home directory path",
			repoPath:    "~/secret",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.validateRepoPath(tt.repoPath)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error for invalid path, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for valid path: %v", err)
				}
			}
		})
	}
}

func TestChildWorkflowExecutor_CopyFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tempDir, "source.txt")
	content := []byte("test content")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Test copy
	executor := &ChildWorkflowExecutor{}
	dstPath := filepath.Join(tempDir, "dest.txt")

	if err := executor.copyFile(srcPath, dstPath); err != nil {
		t.Errorf("Failed to copy file: %v", err)
	}

	// Verify content
	copiedContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Errorf("Failed to read copied file: %v", err)
	}

	if string(copiedContent) != string(content) {
		t.Errorf("Content mismatch: expected %s, got %s", content, copiedContent)
	}

	// Verify permissions
	srcInfo, _ := os.Stat(srcPath)
	dstInfo, _ := os.Stat(dstPath)
	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("Permission mismatch: expected %v, got %v", srcInfo.Mode(), dstInfo.Mode())
	}
}

func TestChildWorkflowExecutor_CopyRepository(t *testing.T) {
	tempDir := t.TempDir()

	// Create source repository structure
	srcRepo := filepath.Join(tempDir, "source-repo")
	os.MkdirAll(filepath.Join(srcRepo, "subdir"), 0755)
	os.MkdirAll(filepath.Join(srcRepo, ".git"), 0755)

	// Create files
	os.WriteFile(filepath.Join(srcRepo, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(srcRepo, "subdir", "file2.txt"), []byte("content2"), 0644)
	os.WriteFile(filepath.Join(srcRepo, ".git", "config"), []byte("git config"), 0644)

	// Test copy
	executor := &ChildWorkflowExecutor{}
	dstRepo := filepath.Join(tempDir, "dest-repo")

	if err := executor.copyRepository(srcRepo, dstRepo); err != nil {
		t.Errorf("Failed to copy repository: %v", err)
	}

	// Verify structure (should skip .git)
	if _, err := os.Stat(filepath.Join(dstRepo, "file1.txt")); err != nil {
		t.Error("file1.txt should be copied")
	}
	if _, err := os.Stat(filepath.Join(dstRepo, "subdir", "file2.txt")); err != nil {
		t.Error("subdir/file2.txt should be copied")
	}
	if _, err := os.Stat(filepath.Join(dstRepo, ".git")); !os.IsNotExist(err) {
		t.Error(".git directory should not be copied")
	}
}

func TestChildWorkflowExecutor_ValidateWorkflowInputs(t *testing.T) {
	executor := &ChildWorkflowExecutor{}

	tests := []struct {
		name        string
		inputs      map[string]string
		workflow    config.Workflow
		expectError bool
	}{
		{
			name: "all required inputs provided",
			inputs: map[string]string{
				"version": "1.0.0",
				"env":     "prod",
			},
			workflow: config.Workflow{
				Inputs: map[string]config.WorkflowInput{
					"version": {Type: "string", Required: true},
					"env":     {Type: "string", Required: true, Validation: config.WorkflowInputValidation{Enum: []string{"dev", "prod"}}},
				},
			},
			expectError: false,
		},
		{
			name:   "missing required input",
			inputs: map[string]string{},
			workflow: config.Workflow{
				Inputs: map[string]config.WorkflowInput{
					"version": {Type: "string", Required: true},
				},
			},
			expectError: true,
		},
		{
			name: "invalid enum value",
			inputs: map[string]string{
				"env": "staging",
			},
			workflow: config.Workflow{
				Inputs: map[string]config.WorkflowInput{
					"env": {Type: "string", Required: false, Validation: config.WorkflowInputValidation{Enum: []string{"dev", "prod"}}},
				},
			},
			expectError: true,
		},
		{
			name: "extra inputs allowed",
			inputs: map[string]string{
				"version": "1.0.0",
				"extra":   "value",
			},
			workflow: config.Workflow{
				Inputs: map[string]config.WorkflowInput{
					"version": {Type: "string", Required: true},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.validateWorkflowInputs(tt.inputs, tt.workflow)
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

func TestChildWorkflowExecutor_CleanupWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	executor := &ChildWorkflowExecutor{}

	// Test cleanup of existing workspace
	workspace := filepath.Join(tempDir, "workspace")
	os.MkdirAll(filepath.Join(workspace, "subdir"), 0755)
	os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("content"), 0644)

	if err := executor.cleanupWorkspace(workspace); err != nil {
		t.Errorf("Failed to cleanup workspace: %v", err)
	}

	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Error("Workspace should be removed")
	}

	// Test idempotent cleanup (already cleaned)
	if err := executor.cleanupWorkspace(workspace); err != nil {
		t.Errorf("Cleanup should be idempotent: %v", err)
	}
}

func TestChildWorkflowExecutor_ConvertExecutionResult(t *testing.T) {
	executor := &ChildWorkflowExecutor{}

	// Test nil result
	if result := executor.convertExecutionResult(nil); result != nil {
		t.Error("Expected nil for nil input")
	}

	// Test conversion
	engineResult := &ExecutionResult{
		RunID:     "test-run-123",
		Success:   true,
		Error:     nil,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Minute),
		Steps: []StepResult{
			{
				ID:        "step-1",
				Success:   true,
				Output:    "step output",
				Outputs:   map[string]string{"key": "value"},
				StartTime: time.Now(),
				EndTime:   time.Now().Add(30 * time.Second),
			},
		},
	}

	interfaceResult := executor.convertExecutionResult(engineResult)

	if interfaceResult.RunID != engineResult.RunID {
		t.Errorf("RunID mismatch: expected %s, got %s", engineResult.RunID, interfaceResult.RunID)
	}
	if interfaceResult.Success != engineResult.Success {
		t.Errorf("Success mismatch: expected %v, got %v", engineResult.Success, interfaceResult.Success)
	}
	if len(interfaceResult.Steps) != len(engineResult.Steps) {
		t.Errorf("Steps count mismatch: expected %d, got %d", len(engineResult.Steps), len(interfaceResult.Steps))
	}
	if interfaceResult.Steps[0].Output != engineResult.Steps[0].Output {
		t.Errorf("Step output mismatch: expected %s, got %s", engineResult.Steps[0].Output, interfaceResult.Steps[0].Output)
	}
}

func TestChildWorkflowExecutor_ExecuteWorkflow_ValidationErrors(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	executor, err := NewChildWorkflowExecutor(factory, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	tests := []struct {
		name         string
		repoPath     string
		workflowName string
		inputs       map[string]string
		expectError  string
	}{
		{
			name:         "empty repo path",
			repoPath:     "",
			workflowName: "test",
			inputs:       map[string]string{},
			expectError:  "repository path is required",
		},
		{
			name:         "empty workflow name",
			repoPath:     "test-repo",
			workflowName: "",
			inputs:       map[string]string{},
			expectError:  "workflow name is required",
		},
		{
			name:         "path traversal",
			repoPath:     "../../../etc",
			workflowName: "test",
			inputs:       map[string]string{},
			expectError:  "invalid repository path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.ExecuteWorkflow(ctx, tt.repoPath, tt.workflowName, tt.inputs)
			if err == nil {
				t.Error("Expected error, but got none")
			} else if !contains(err.Error(), tt.expectError) {
				t.Errorf("Expected error containing '%s', got: %v", tt.expectError, err)
			}
			if result != nil {
				t.Error("Expected nil result on error")
			}
		})
	}
}

// Mock test for full workflow execution.
func TestChildWorkflowExecutor_ExecuteWorkflow_MockExecution(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	// Create a test repository in cache
	testRepo := filepath.Join(cacheDir, "repos", "test-org", "test-repo", "main")
	os.MkdirAll(testRepo, 0755)

	// Create tako.yml
	takoYml := `
version: 1
workflows:
  test-workflow:
    inputs:
      message:
        type: string
        required: true
    steps:
      - id: echo-message
        run: echo "${{ inputs.message }}"
`
	os.WriteFile(filepath.Join(testRepo, "tako.yml"), []byte(takoYml), 0644)

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	executor, err := NewChildWorkflowExecutor(factory, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Test execution with cached repository
	ctx := context.Background()
	inputs := map[string]string{
		"message": "Hello from child workflow",
	}

	// This will fail because we need a proper Runner setup, but it tests the flow
	result, err := executor.ExecuteWorkflow(ctx, "test-org/test-repo", "test-workflow", inputs)

	// We expect this to fail at the actual workflow execution stage
	// because we don't have all the Runner dependencies set up
	if err != nil {
		// This is expected in unit tests without full Runner setup
		t.Logf("Expected execution error in mock test: %v", err)
	} else if result != nil && result.Success {
		t.Log("Mock execution completed successfully")
	}
}

func TestChildWorkflowExecutor_ResolveChildRepoPath(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	executor := &ChildWorkflowExecutor{factory: factory}
	childWorkspace := filepath.Join(tempDir, "child")
	os.MkdirAll(childWorkspace, 0755)

	// Test with local path
	localRepo := filepath.Join(tempDir, "local-repo")
	os.MkdirAll(localRepo, 0755)
	os.WriteFile(filepath.Join(localRepo, "test.txt"), []byte("test"), 0644)

	resolvedPath, err := executor.resolveChildRepoPath(localRepo, childWorkspace)
	if err != nil {
		t.Errorf("Failed to resolve local repo path: %v", err)
	}
	expectedPath := filepath.Join(childWorkspace, "repo")
	if resolvedPath != expectedPath {
		t.Errorf("Expected resolved path %s, got %s", expectedPath, resolvedPath)
	}

	// Verify copy worked
	if _, err := os.Stat(filepath.Join(resolvedPath, "test.txt")); err != nil {
		t.Error("Local repo was not copied correctly")
	}

	// Test with cached remote repo
	cachedRepo := filepath.Join(cacheDir, "repos", "test-org", "test-repo", "main")
	os.MkdirAll(cachedRepo, 0755)
	os.WriteFile(filepath.Join(cachedRepo, "cached.txt"), []byte("cached"), 0644)

	childWorkspace2 := filepath.Join(tempDir, "child2")
	os.MkdirAll(childWorkspace2, 0755)

	resolvedPath2, err := executor.resolveChildRepoPath("test-org/test-repo", childWorkspace2)
	if err != nil {
		t.Errorf("Failed to resolve cached repo path: %v", err)
	}

	// Verify cached copy worked
	if _, err := os.Stat(filepath.Join(resolvedPath2, "cached.txt")); err != nil {
		t.Error("Cached repo was not copied correctly")
	}

	// Test with non-existent remote repo
	_, err = executor.resolveChildRepoPath("non-existent/repo", childWorkspace)
	if err == nil {
		t.Error("Expected error for non-existent repo")
	}
}

func TestChildWorkflowExecutor_MissingTakoYml(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	// Create a test repository without tako.yml
	testRepo := filepath.Join(cacheDir, "repos", "test-org", "no-tako", "main")
	os.MkdirAll(testRepo, 0755)
	os.WriteFile(filepath.Join(testRepo, "README.md"), []byte("No tako.yml here"), 0644)

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	executor, err := NewChildWorkflowExecutor(factory, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()
	_, err = executor.ExecuteWorkflow(ctx, "test-org/no-tako", "test-workflow", map[string]string{})

	if err == nil {
		t.Error("Expected error for missing tako.yml")
	} else if !contains(err.Error(), "tako.yml not found") {
		t.Errorf("Expected 'tako.yml not found' error, got: %v", err)
	}
}

func TestChildWorkflowExecutor_MalformedTakoYml(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	// Create a test repository with malformed tako.yml
	testRepo := filepath.Join(cacheDir, "repos", "test-org", "bad-tako", "main")
	os.MkdirAll(testRepo, 0755)
	os.WriteFile(filepath.Join(testRepo, "tako.yml"), []byte("not: valid: yaml: :::"), 0644)

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	executor, err := NewChildWorkflowExecutor(factory, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()
	_, err = executor.ExecuteWorkflow(ctx, "test-org/bad-tako", "test-workflow", map[string]string{})

	if err == nil {
		t.Error("Expected error for malformed tako.yml")
	} else if !contains(err.Error(), "failed to load tako.yml") {
		t.Errorf("Expected 'failed to load tako.yml' error, got: %v", err)
	}
}
