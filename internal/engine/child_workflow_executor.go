package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/interfaces"
)

// ChildWorkflowExecutor executes workflows in isolated child environments.
// It implements the interfaces.WorkflowRunner interface to enable dependency injection
// in the fan-out executor.
type ChildWorkflowExecutor struct {
	factory          *ChildRunnerFactory
	templateEngine   *TemplateEngine
	containerManager *ContainerManager
	resourceManager  *ResourceManager

	// Synchronization
	mu sync.RWMutex
}

// NewChildWorkflowExecutor creates a new executor for child workflows.
// It uses the provided factory to create isolated Runner instances.
func NewChildWorkflowExecutor(factory *ChildRunnerFactory, templateEngine *TemplateEngine, containerManager *ContainerManager, resourceManager *ResourceManager) (*ChildWorkflowExecutor, error) {
	if factory == nil {
		return nil, fmt.Errorf("child runner factory is required")
	}

	return &ChildWorkflowExecutor{
		factory:          factory,
		templateEngine:   templateEngine,
		containerManager: containerManager,
		resourceManager:  resourceManager,
	}, nil
}

// ExecuteWorkflow executes a workflow in an isolated child environment.
// It implements the interfaces.WorkflowRunner interface.
func (e *ChildWorkflowExecutor) ExecuteWorkflow(ctx context.Context, repoPath, workflowName string, inputs map[string]string) (*interfaces.ExecutionResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Validate inputs
	if repoPath == "" {
		return nil, fmt.Errorf("repository path is required")
	}
	if workflowName == "" {
		return nil, fmt.Errorf("workflow name is required")
	}

	// Security validation: prevent path traversal
	if err := e.validateRepoPath(repoPath); err != nil {
		return nil, fmt.Errorf("invalid repository path: %w", err)
	}

	// Create isolated child runner
	childRunner, childWorkspace, err := e.factory.CreateChildRunner()
	if err != nil {
		return nil, fmt.Errorf("failed to create child runner: %w", err)
	}

	// Ensure cleanup of child workspace
	defer func() {
		// Close the runner first
		if closeErr := childRunner.Close(); closeErr != nil {
			// Log error but don't override the main error
			fmt.Fprintf(os.Stderr, "warning: failed to close child runner: %v\n", closeErr)
		}

		// Clean up the workspace directory
		if cleanErr := e.cleanupWorkspace(childWorkspace); cleanErr != nil {
			// Log error but don't override the main error
			fmt.Fprintf(os.Stderr, "warning: failed to cleanup child workspace %s: %v\n", childWorkspace, cleanErr)
		}
	}()

	// Resolve repository path to child workspace
	childRepoPath, err := e.resolveChildRepoPath(repoPath, childWorkspace)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve child repository path: %w", err)
	}

	// Discover tako.yml in the child repository
	takoYmlPath := filepath.Join(childRepoPath, "tako.yml")
	if _, err := os.Stat(takoYmlPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("tako.yml not found in repository %s", repoPath)
		}
		return nil, fmt.Errorf("failed to access tako.yml: %w", err)
	}

	// Load and validate the configuration
	cfg, err := config.Load(takoYmlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load tako.yml: %w", err)
	}

	// Find the requested workflow
	workflow, exists := cfg.Workflows[workflowName]
	if !exists {
		return nil, fmt.Errorf("workflow '%s' not found in repository %s", workflowName, repoPath)
	}

	// Validate workflow inputs against the workflow definition
	if err := e.validateWorkflowInputs(inputs, workflow); err != nil {
		return nil, fmt.Errorf("invalid workflow inputs: %w", err)
	}

	// Execute the workflow using the child runner
	result, err := childRunner.ExecuteWorkflow(ctx, workflowName, inputs, childRepoPath)
	if err != nil {
		return nil, fmt.Errorf("workflow execution failed: %w", err)
	}

	// Convert engine.ExecutionResult to interfaces.ExecutionResult
	return e.convertExecutionResult(result), nil
}

// validateRepoPath validates the repository path to prevent path traversal attacks.
func (e *ChildWorkflowExecutor) validateRepoPath(repoPath string) error {
	// Check for path traversal patterns
	if strings.Contains(repoPath, "..") {
		return fmt.Errorf("path traversal detected")
	}

	// Check for absolute paths (we expect relative paths or repo names)
	if filepath.IsAbs(repoPath) {
		return fmt.Errorf("absolute paths not allowed")
	}

	// Additional validation for repository format
	if strings.HasPrefix(repoPath, "/") || strings.HasPrefix(repoPath, "~") {
		return fmt.Errorf("invalid repository path format")
	}

	return nil
}

// resolveChildRepoPath resolves the repository path within the child workspace.
// It handles both local paths and remote repository references.
func (e *ChildWorkflowExecutor) resolveChildRepoPath(repoPath, childWorkspace string) (string, error) {
	// Check if it's a local path
	if _, err := os.Stat(repoPath); err == nil {
		// It's a local path, copy it to child workspace
		childRepoPath := filepath.Join(childWorkspace, "repo")
		if err := e.copyRepository(repoPath, childRepoPath); err != nil {
			return "", fmt.Errorf("failed to copy repository: %w", err)
		}
		return childRepoPath, nil
	}

	// It's likely a remote repository reference (owner/repo format)
	// Validate the repository format first
	if !strings.Contains(repoPath, "/") {
		return "", fmt.Errorf("invalid repository format: %s", repoPath)
	}

	// Parse repository parts
	repoParts := strings.Split(repoPath, "/")
	if len(repoParts) < 2 {
		return "", fmt.Errorf("invalid repository format: %s", repoPath)
	}

	childRepoPath := filepath.Join(childWorkspace, "repo")

	// Try to find in cache first (assume main branch for now)
	cachedPath := filepath.Join(e.factory.cacheDir, "repos", repoPath, "main")
	if _, err := os.Stat(cachedPath); err == nil {
		// Found in cache, copy it
		if err := e.copyRepository(cachedPath, childRepoPath); err != nil {
			return "", fmt.Errorf("failed to copy from cache: %w", err)
		}
		return childRepoPath, nil
	}

	// Not in cache, would need to clone - for now return error
	return "", fmt.Errorf("repository %s not found in cache", repoPath)
}

// copyRepository copies a repository from source to destination.
func (e *ChildWorkflowExecutor) copyRepository(src, dst string) error {
	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	// For now, we'll do a simple directory walk and copy
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		// Skip .git directory
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		// Create directories
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy files
		return e.copyFile(path, dstPath)
	})
}

// copyFile copies a single file from source to destination.
func (e *ChildWorkflowExecutor) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Get source file info for permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Copy content
	buf := make([]byte, 64*1024) // 64KB buffer
	for {
		n, err := sourceFile.Read(buf)
		if n > 0 {
			if _, werr := destFile.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return err
		}
	}

	// Set permissions
	return os.Chmod(dst, srcInfo.Mode())
}

// validateWorkflowInputs validates the provided inputs against the workflow definition.
func (e *ChildWorkflowExecutor) validateWorkflowInputs(inputs map[string]string, workflow config.Workflow) error {
	// Check required inputs
	for name, input := range workflow.Inputs {
		if input.Required {
			if _, exists := inputs[name]; !exists {
				return fmt.Errorf("required input '%s' not provided", name)
			}
		}
	}

	// Validate input types and constraints
	for name, value := range inputs {
		inputDef, exists := workflow.Inputs[name]
		if !exists {
			// Extra inputs are allowed, just skip validation
			continue
		}

		// Validate based on type
		switch inputDef.Type {
		case "string":
			// Validate enum if specified
			if len(inputDef.Validation.Enum) > 0 {
				valid := false
				for _, enumVal := range inputDef.Validation.Enum {
					if value == enumVal {
						valid = true
						break
					}
				}
				if !valid {
					return fmt.Errorf("input '%s' must be one of: %v", name, inputDef.Validation.Enum)
				}
			}
		case "number":
			// For now, we're passing everything as strings, so skip number validation
			// In a full implementation, you'd parse and validate numeric constraints
		}
	}

	return nil
}

// cleanupWorkspace removes the child workspace directory.
// It's designed to be idempotent - safe to call multiple times.
func (e *ChildWorkflowExecutor) cleanupWorkspace(workspace string) error {
	// Check if workspace exists
	if _, err := os.Stat(workspace); os.IsNotExist(err) {
		// Already cleaned up, nothing to do
		return nil
	}

	// Remove the workspace
	if err := os.RemoveAll(workspace); err != nil {
		// Check if it's because it doesn't exist (race condition)
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to remove workspace: %w", err)
	}

	return nil
}

// convertExecutionResult converts engine.ExecutionResult to interfaces.ExecutionResult.
func (e *ChildWorkflowExecutor) convertExecutionResult(result *ExecutionResult) *interfaces.ExecutionResult {
	if result == nil {
		return nil
	}

	// Convert step results
	steps := make([]interfaces.StepResult, len(result.Steps))
	for i, step := range result.Steps {
		steps[i] = interfaces.StepResult{
			ID:        step.ID,
			Success:   step.Success,
			Error:     step.Error,
			StartTime: step.StartTime,
			EndTime:   step.EndTime,
			Output:    step.Output,
			Outputs:   step.Outputs,
		}
	}

	return &interfaces.ExecutionResult{
		RunID:     result.RunID,
		Success:   result.Success,
		Error:     result.Error,
		StartTime: result.StartTime,
		EndTime:   result.EndTime,
		Steps:     steps,
	}
}
