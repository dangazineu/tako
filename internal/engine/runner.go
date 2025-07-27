package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/interfaces"
	"github.com/dangazineu/tako/internal/steps"
)

// ExecutionMode defines how the workflow should be executed.
type ExecutionMode int

const (
	ExecutionModeNormal ExecutionMode = iota
	ExecutionModeDryRun
	ExecutionModeDebug
)

// ExecutionResult represents the result of a workflow execution.
type ExecutionResult struct {
	RunID     string
	Success   bool
	Error     error
	StartTime time.Time
	EndTime   time.Time
	Steps     []StepResult
}

// StepResult represents the result of a single step execution.
type StepResult struct {
	ID        string
	Success   bool
	Error     error
	StartTime time.Time
	EndTime   time.Time
	Output    string
	Outputs   map[string]string
}

// Runner executes workflows with comprehensive state management and workspace isolation.
type Runner struct {
	mode          ExecutionMode
	workspaceRoot string
	cacheDir      string

	// Execution tree management
	runID string
	state *ExecutionState
	locks *LockManager

	// Template processing
	templateEngine *TemplateEngine

	// Container management
	containerManager *ContainerManager

	// Resource management
	resourceManager *ResourceManager

	// Multi-repository orchestration
	orchestrator *Orchestrator

	// Configuration
	maxConcurrentRepos int
	dryRun             bool
	debug              bool
	noCache            bool
	environment        []string

	// Synchronization
	mu sync.RWMutex
}

var _ interfaces.WorkflowRunner = (*Runner)(nil)

// NewRunner creates a new execution runner with the specified configuration.
func NewRunner(opts RunnerOptions) (*Runner, error) {
	runID := GenerateRunID()

	// Use the provided workspace root
	workspaceRoot := opts.WorkspaceRoot
	if workspaceRoot == "" {
		return nil, fmt.Errorf("workspace root is required")
	}

	// Create workspace directory
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %v", err)
	}

	// Initialize state manager
	state, err := NewExecutionState(runID, workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize execution state: %v", err)
	}

	// Initialize lock manager
	locks, err := NewLockManager(filepath.Join(workspaceRoot, "locks"))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize lock manager: %v", err)
	}

	// Initialize container manager (optional - only log warning if unavailable)
	containerManager, err := NewContainerManager(opts.Debug)
	if err != nil {
		// Container runtime is optional - log warning but continue
		if opts.Debug {
			fmt.Printf("Warning: Container runtime not available: %v\n", err)
		}
		containerManager = nil
	}

	// Initialize resource manager
	resourceConfig := &ResourceManagerConfig{
		WarningThreshold:   0.9, // 90% warning threshold
		MonitoringInterval: 30 * time.Second,
		MaxHistoryEntries:  100,
		Debug:              opts.Debug,
	}
	resourceManager := NewResourceManager(resourceConfig)

	// Initialize orchestrator
	orchestrator := NewOrchestrator(workspaceRoot, opts.CacheDir)

	mode := ExecutionModeNormal
	if opts.DryRun {
		mode = ExecutionModeDryRun
	} else if opts.Debug {
		mode = ExecutionModeDebug
	}

	return &Runner{
		mode:               mode,
		workspaceRoot:      workspaceRoot,
		cacheDir:           opts.CacheDir,
		runID:              runID,
		state:              state,
		locks:              locks,
		templateEngine:     NewTemplateEngine(),
		containerManager:   containerManager,
		resourceManager:    resourceManager,
		orchestrator:       orchestrator,
		maxConcurrentRepos: opts.MaxConcurrentRepos,
		dryRun:             opts.DryRun,
		debug:              opts.Debug,
		noCache:            opts.NoCache,
		environment:        opts.Environment,
	}, nil
}

// RunnerOptions configures the execution runner.
type RunnerOptions struct {
	WorkspaceRoot      string
	CacheDir           string
	MaxConcurrentRepos int
	DryRun             bool
	Debug              bool
	NoCache            bool
	Environment        []string // Environment variables for command execution
}

// ExecuteWorkflow executes a workflow in single-repository mode.
func (r *Runner) ExecuteWorkflow(ctx context.Context, workflowName string, inputs map[string]string, repoPath string) (*ExecutionResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	startTime := time.Now()

	// Load workflow configuration
	configPath := filepath.Join(repoPath, "tako.yml")
	cfg, err := config.Load(configPath)
	if err != nil {
		return &ExecutionResult{
			RunID:     r.runID,
			Success:   false,
			Error:     fmt.Errorf("failed to load config: %v", err),
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err
	}

	// Find the specified workflow
	workflow, exists := cfg.Workflows[workflowName]
	if !exists {
		err := fmt.Errorf("workflow '%s' not found", workflowName)
		return &ExecutionResult{
			RunID:     r.runID,
			Success:   false,
			Error:     err,
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err
	}

	// Validate inputs
	if err := r.validateInputs(workflow, inputs); err != nil {
		return &ExecutionResult{
			RunID:     r.runID,
			Success:   false,
			Error:     fmt.Errorf("input validation failed: %v", err),
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err
	}

	// Update execution state
	if err := r.state.StartExecution(workflowName, repoPath, inputs); err != nil {
		return &ExecutionResult{
			RunID:     r.runID,
			Success:   false,
			Error:     fmt.Errorf("failed to start execution: %v", err),
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err
	}

	// Execute workflow steps
	stepResults, err := r.executeSteps(ctx, workflow.Steps, repoPath, inputs)

	endTime := time.Now()
	success := err == nil

	// Update final state
	if success {
		r.state.CompleteExecution()
	} else {
		r.state.FailExecution(err.Error())
	}

	return &ExecutionResult{
		RunID:     r.runID,
		Success:   success,
		Error:     err,
		StartTime: startTime,
		EndTime:   endTime,
		Steps:     stepResults,
	}, err
}

// ExecuteMultiRepoWorkflow executes a workflow with multi-repository orchestration.
func (r *Runner) ExecuteMultiRepoWorkflow(ctx context.Context, workflowName string, inputs map[string]string, parentRepo string) (*ExecutionResult, error) {
	// For now, implement basic multi-repository execution by resolving the repo path
	// and delegating to single-repository execution
	// TODO: Implement full multi-repository execution with event-driven orchestration
	// This will be the main orchestration logic that handles:
	// 1. Parent repository workflow execution
	// 2. Event emission via tako/fan-out@v1 steps
	// 3. Child repository subscription evaluation
	// 4. Parallel execution of child workflows
	// 5. State synchronization across all repositories

	// Parse repository specification (e.g., "owner/repo:branch")
	repoPath, err := r.resolveRepositoryPath(parentRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve repository path: %v", err)
	}

	// Delegate to single-repository execution for now
	return r.ExecuteWorkflow(ctx, workflowName, inputs, repoPath)
}

// resolveRepositoryPath resolves a repository specification to a local path.
func (r *Runner) resolveRepositoryPath(repoSpec string) (string, error) {
	// Parse repository specification: "owner/repo:branch" or "owner/repo"
	parts := strings.Split(repoSpec, ":")
	repoPath := parts[0]
	branch := "main"
	if len(parts) > 1 {
		branch = parts[1]
	}

	// Split owner/repo
	ownerRepo := strings.Split(repoPath, "/")
	if len(ownerRepo) != 2 {
		return "", fmt.Errorf("invalid repository specification: %s (expected format: owner/repo or owner/repo:branch)", repoSpec)
	}

	owner := ownerRepo[0]
	repo := ownerRepo[1]

	// Construct cache path: ~/.tako/cache/repos/owner/repo/branch
	cachePath := filepath.Join(r.cacheDir, "repos", owner, repo, branch)

	// Check if repository exists in cache
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return "", fmt.Errorf("repository %s not found in cache at %s", repoSpec, cachePath)
	}

	return cachePath, nil
}

// Resume resumes a previously failed or interrupted execution.
func (r *Runner) Resume(ctx context.Context, runID string) (*ExecutionResult, error) {
	// TODO: Implement execution resume functionality
	// This will handle:
	// 1. Loading previous execution state
	// 2. Determining which steps need to be re-executed
	// 3. Resuming from the appropriate point
	// 4. Smart partial resume for failed branches only

	return nil, fmt.Errorf("execution resume not yet implemented")
}

// validateInputs validates workflow inputs against the schema.
func (r *Runner) validateInputs(workflow config.Workflow, inputs map[string]string) error {
	for name, input := range workflow.Inputs {
		value, provided := inputs[name]

		// Check required inputs
		if input.Required && !provided {
			return fmt.Errorf("required input '%s' not provided", name)
		}

		// Use default if not provided
		if !provided && input.Default != nil {
			inputs[name] = fmt.Sprintf("%v", input.Default)
			continue
		}

		// Validate provided value
		if provided {
			if err := r.validateInputValue(name, input, value); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateInputValue validates a single input value against its schema.
func (r *Runner) validateInputValue(name string, input config.WorkflowInput, value string) error {
	// Type validation would go here
	// For now, we'll implement basic enum validation
	if len(input.Validation.Enum) > 0 {
		for _, validValue := range input.Validation.Enum {
			if value == validValue {
				return nil
			}
		}
		return fmt.Errorf("input '%s' value '%s' is not in allowed values %v", name, value, input.Validation.Enum)
	}

	return nil
}

// executeSteps executes a list of workflow steps.
func (r *Runner) executeSteps(ctx context.Context, steps []config.WorkflowStep, workDir string, inputs map[string]string) ([]StepResult, error) {
	var results []StepResult
	stepOutputs := make(map[string]map[string]string)

	for _, step := range steps {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result, err := r.executeStep(ctx, step, workDir, inputs, stepOutputs)
		results = append(results, result)

		if err != nil {
			return results, fmt.Errorf("step '%s' failed: %v", step.ID, err)
		}

		// Store step outputs for future steps
		if len(result.Outputs) > 0 {
			stepOutputs[step.ID] = result.Outputs
		}
	}

	return results, nil
}

// executeStep executes a single workflow step.
func (r *Runner) executeStep(ctx context.Context, step config.WorkflowStep, workDir string, inputs map[string]string, stepOutputs map[string]map[string]string) (StepResult, error) {
	startTime := time.Now()
	stepID := step.ID
	if stepID == "" {
		stepID = fmt.Sprintf("step-%d", time.Now().UnixNano())
	}

	// Update state to track step execution
	if err := r.state.StartStep(stepID); err != nil {
		return StepResult{
			ID:        stepID,
			Success:   false,
			Error:     fmt.Errorf("failed to start step tracking: %v", err),
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err
	}

	// Handle dry run mode
	if r.mode == ExecutionModeDryRun {
		output := fmt.Sprintf("[dry-run] %s", step.Run)

		// Simulate step completion in state
		r.state.CompleteStep(stepID, output, nil)

		return StepResult{
			ID:        stepID,
			Success:   true,
			StartTime: startTime,
			EndTime:   time.Now(),
			Output:    output,
		}, nil
	}

	// Check if this is a built-in step (uses: field)
	if step.Uses != "" {
		return r.executeBuiltinStep(ctx, step, stepID, startTime)
	}

	// Check if this is a container step (image: field)
	if IsContainerStep(step) {
		return r.executeContainerStep(ctx, step, stepID, workDir, inputs, stepOutputs, startTime)
	}

	// Execute shell command
	return r.executeShellStep(ctx, step, stepID, workDir, inputs, stepOutputs, startTime)
}

// executeShellStep executes a step with a shell command.
func (r *Runner) executeShellStep(ctx context.Context, step config.WorkflowStep, stepID, workDir string, inputs map[string]string, stepOutputs map[string]map[string]string, startTime time.Time) (StepResult, error) {
	// Expand template variables in the command
	command, err := r.expandTemplate(step.Run, inputs, stepOutputs)
	if err != nil {
		r.state.FailStep(stepID, fmt.Sprintf("template expansion failed: %v", err))
		return StepResult{
			ID:        stepID,
			Success:   false,
			Error:     fmt.Errorf("template expansion failed: %v", err),
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err
	}

	// Create command with proper context cancellation
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workDir

	// Set up environment variables
	env := r.getEnvironment()
	cmd.Env = append(env,
		fmt.Sprintf("TAKO_RUN_ID=%s", r.runID),
		fmt.Sprintf("TAKO_STEP_ID=%s", stepID),
		fmt.Sprintf("TAKO_WORKSPACE=%s", r.workspaceRoot),
	)

	// Add inputs as environment variables
	for key, value := range inputs {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TAKO_INPUT_%s=%s", strings.ToUpper(key), value))
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command
	err = cmd.Run()

	endTime := time.Now()
	output := stdout.String()
	errorOutput := stderr.String()

	// Process outputs if step produces them
	stepOutputValues := make(map[string]string)
	if step.Produces != nil && step.Produces.Outputs != nil {
		for outputName, outputFormat := range step.Produces.Outputs {
			switch outputFormat {
			case "from_stdout":
				stepOutputValues[outputName] = strings.TrimSpace(output)
			case "from_stderr":
				stepOutputValues[outputName] = strings.TrimSpace(errorOutput)
			default:
				// Try to extract from stdout using the format as a regex
				if re, regexErr := regexp.Compile(outputFormat); regexErr == nil {
					matches := re.FindStringSubmatch(output)
					if len(matches) > 1 {
						stepOutputValues[outputName] = matches[1]
					}
				}
			}
		}
	}

	// Update state based on execution result
	if err != nil {
		fullError := fmt.Sprintf("command failed: %v", err)
		if errorOutput != "" {
			fullError = fmt.Sprintf("%s\nstderr: %s", fullError, errorOutput)
		}

		r.state.FailStep(stepID, fullError)

		return StepResult{
			ID:        stepID,
			Success:   false,
			Error:     fmt.Errorf("command execution failed: %v", err),
			StartTime: startTime,
			EndTime:   endTime,
			Output:    output,
			Outputs:   stepOutputValues,
		}, err
	}

	// Step succeeded
	r.state.CompleteStep(stepID, output, stepOutputValues)

	return StepResult{
		ID:        stepID,
		Success:   true,
		StartTime: startTime,
		EndTime:   endTime,
		Output:    output,
		Outputs:   stepOutputValues,
	}, nil
}

// executeBuiltinStep executes a built-in Tako step.
func (r *Runner) executeBuiltinStep(ctx context.Context, step config.WorkflowStep, stepID string, startTime time.Time) (StepResult, error) {
	// Parse step name and version
	stepParts := strings.Split(step.Uses, "@")
	if len(stepParts) != 2 {
		err := fmt.Errorf("invalid built-in step format: %s", step.Uses)
		r.state.FailStep(stepID, err.Error())
		return StepResult{
			ID:        stepID,
			Success:   false,
			Error:     err,
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err
	}

	stepName := stepParts[0]
	stepVersion := stepParts[1]

	switch stepName {
	case "tako/fan-out":
		if stepVersion == "v1" {
			return r.executeFanOutStep(ctx, step, stepID, startTime)
		}
		err := fmt.Errorf("unsupported fan-out step version: %s", stepVersion)
		r.state.FailStep(stepID, err.Error())
		return StepResult{
			ID:        stepID,
			Success:   false,
			Error:     err,
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err

	default:
		err := fmt.Errorf("built-in step not yet implemented: %s", step.Uses)
		r.state.FailStep(stepID, err.Error())
		return StepResult{
			ID:        stepID,
			Success:   false,
			Error:     err,
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err
	}
}

// executeFanOutStep executes a tako/fan-out@v1 step.
func (r *Runner) executeFanOutStep(ctx context.Context, step config.WorkflowStep, stepID string, startTime time.Time) (StepResult, error) {
	// Create fan-out executor
	fanOutExecutor := steps.NewFanOutExecutor(r.orchestrator, r)

	// For now, we need to determine the artifact reference and event payload
	// In a real implementation, this would come from the workflow context
	// TODO: Extract artifact reference from current repository context
	artifactRef := "placeholder/artifact:placeholder" // This should be determined from context
	eventPayload := map[string]string{}               // This should come from step outputs and inputs

	// Execute the fan-out step
	result, err := fanOutExecutor.Execute(ctx, step, artifactRef, eventPayload)
	if err != nil {
		r.state.FailStep(stepID, err.Error())
		return StepResult{
			ID:        stepID,
			Success:   false,
			Error:     err,
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err
	}

	// Mark step as successful
	output := fmt.Sprintf("Fan-out completed: emitted event, triggered %d workflows", len(result.TriggeredWorkflows))
	r.state.CompleteStep(stepID, output, map[string]string{
		"event_emitted":       fmt.Sprintf("%t", result.EventEmitted),
		"triggered_workflows": fmt.Sprintf("%d", len(result.TriggeredWorkflows)),
		"child_run_ids":       strings.Join(result.ChildRunIDs, ","),
	})

	return StepResult{
		ID:        stepID,
		Success:   true,
		StartTime: startTime,
		EndTime:   time.Now(),
		Output:    fmt.Sprintf("Fan-out completed: emitted event, triggered %d workflows", len(result.TriggeredWorkflows)),
		Outputs: map[string]string{
			"event_emitted":       fmt.Sprintf("%t", result.EventEmitted),
			"triggered_workflows": fmt.Sprintf("%d", len(result.TriggeredWorkflows)),
			"child_run_ids":       strings.Join(result.ChildRunIDs, ","),
		},
	}, nil
}

// executeContainerStep executes a step in a container.
func (r *Runner) executeContainerStep(ctx context.Context, step config.WorkflowStep, stepID, workDir string, inputs map[string]string, stepOutputs map[string]map[string]string, startTime time.Time) (StepResult, error) {
	// Check if container manager is available
	if r.containerManager == nil {
		err := fmt.Errorf("container execution requested but no container runtime is available")
		r.state.FailStep(stepID, err.Error())
		return StepResult{
			ID:        stepID,
			Success:   false,
			Error:     err,
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err
	}

	// Expand template variables in the command
	command := step.Run
	if command != "" {
		expandedCommand, err := r.expandTemplate(command, inputs, stepOutputs)
		if err != nil {
			r.state.FailStep(stepID, fmt.Sprintf("template expansion failed: %v", err))
			return StepResult{
				ID:        stepID,
				Success:   false,
				Error:     fmt.Errorf("template expansion failed: %v", err),
				StartTime: startTime,
				EndTime:   time.Now(),
			}, err
		}
		command = expandedCommand
	}

	// Create a modified step with expanded command for container config
	containerStep := step
	containerStep.Run = command

	// Build container configuration
	env := r.getEnvironment()
	envMap := make(map[string]string)
	for _, envVar := range env {
		if parts := strings.SplitN(envVar, "=", 2); len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Add Tako-specific environment variables
	envMap["TAKO_RUN_ID"] = r.runID
	envMap["TAKO_STEP_ID"] = stepID
	envMap["TAKO_WORKSPACE"] = r.workspaceRoot

	// Add inputs as environment variables
	for key, value := range inputs {
		envMap[fmt.Sprintf("TAKO_INPUT_%s", strings.ToUpper(key))] = value
	}

	// Get repository name from work directory for resource validation
	repoName := r.getRepositoryNameFromPath(workDir)

	// Validate resource requests if resource manager is available
	if r.resourceManager != nil {
		// Extract resource requests from step (if any)
		cpuRequest := ""
		memoryRequest := ""

		// Parse resource requirements if specified in step configuration
		if step.Resources != nil {
			cpuRequest = step.Resources.CPULimit
			memoryRequest = step.Resources.MemLimit
		}

		// Validate resource request against hierarchical limits
		if err := r.resourceManager.ValidateResourceRequest(repoName, stepID, cpuRequest, memoryRequest); err != nil {
			r.state.FailStep(stepID, fmt.Sprintf("resource validation failed: %v", err))
			return StepResult{
				ID:        stepID,
				Success:   false,
				Error:     fmt.Errorf("resource validation failed: %v", err),
				StartTime: startTime,
				EndTime:   time.Now(),
			}, err
		}
	}

	// Build container configuration with resource limits
	var resources *config.Resources
	if step.Resources != nil {
		resources = step.Resources
	}

	containerConfig, err := r.containerManager.BuildContainerConfig(containerStep, workDir, envMap, resources)
	if err != nil {
		r.state.FailStep(stepID, fmt.Sprintf("container configuration failed: %v", err))
		return StepResult{
			ID:        stepID,
			Success:   false,
			Error:     fmt.Errorf("container configuration failed: %v", err),
			StartTime: startTime,
			EndTime:   time.Now(),
		}, err
	}

	// Pull image if needed (with retry for network issues)
	pullCtx, pullCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer pullCancel()

	if err := r.containerManager.PullImage(pullCtx, step.Image); err != nil {
		// Log warning but continue if image might be available locally
		if r.debug {
			fmt.Printf("Warning: failed to pull image %s: %v\n", step.Image, err)
		}
	}

	// Execute container
	result, err := r.containerManager.RunContainer(ctx, containerConfig, stepID)
	endTime := time.Now()

	if err != nil {
		r.state.FailStep(stepID, fmt.Sprintf("container execution failed: %v", err))
		return StepResult{
			ID:        stepID,
			Success:   false,
			Error:     fmt.Errorf("container execution failed: %v", err),
			StartTime: startTime,
			EndTime:   endTime,
			Output:    result.Stderr, // Include stderr in output for debugging
		}, err
	}

	// Combine stdout and stderr for output
	output := result.Stdout
	if result.Stderr != "" {
		output = fmt.Sprintf("%s\nSTDERR:\n%s", result.Stdout, result.Stderr)
	}

	// Check exit code
	if result.ExitCode != 0 {
		err := fmt.Errorf("container exited with code %d", result.ExitCode)
		r.state.FailStep(stepID, fmt.Sprintf("container failed with exit code %d", result.ExitCode))
		return StepResult{
			ID:        stepID,
			Success:   false,
			Error:     err,
			StartTime: startTime,
			EndTime:   endTime,
			Output:    output,
		}, err
	}

	// Process outputs if configured
	stepOutputValues := make(map[string]string)
	if step.Produces != nil && step.Produces.Outputs != nil {
		for outputName, outputType := range step.Produces.Outputs {
			switch outputType {
			case "from_stdout":
				stepOutputValues[outputName] = strings.TrimSpace(result.Stdout)
			case "from_stderr":
				stepOutputValues[outputName] = strings.TrimSpace(result.Stderr)
			default:
				// For other output types, would need to implement file reading, etc.
				stepOutputValues[outputName] = strings.TrimSpace(result.Stdout)
			}
		}
	}

	// Step succeeded
	r.state.CompleteStep(stepID, output, stepOutputValues)

	return StepResult{
		ID:        stepID,
		Success:   true,
		StartTime: startTime,
		EndTime:   endTime,
		Output:    output,
		Outputs:   stepOutputValues,
	}, nil
}

// expandTemplate expands template variables in a string using the enhanced template engine.
func (r *Runner) expandTemplate(tmplStr string, inputs map[string]string, stepOutputs map[string]map[string]string) (string, error) {
	// Build template context
	context := NewContextBuilder().
		WithInputs(inputs).
		WithStepOutputs(stepOutputs).
		Build()

	// Use the enhanced template engine
	return r.templateEngine.ExpandTemplate(tmplStr, context)
}

// GetRunID returns the current run ID.
func (r *Runner) GetRunID() string {
	return r.runID
}

// GetWorkspaceRoot returns the workspace root directory.
func (r *Runner) GetWorkspaceRoot() string {
	return r.workspaceRoot
}

// getEnvironment returns the environment variables for command execution.
func (r *Runner) getEnvironment() []string {
	if r.environment != nil {
		return r.environment
	}
	// Return an empty environment if none provided
	return []string{}
}

// getRepositoryNameFromPath extracts repository name from work directory path.
func (r *Runner) getRepositoryNameFromPath(workDir string) string {
	// Extract repository name from path like /cache/repos/owner/repo/branch
	// or use a fallback based on the work directory
	parts := strings.Split(filepath.Clean(workDir), string(filepath.Separator))

	// Look for the pattern .../repos/owner/repo/...
	for i, part := range parts {
		if part == "repos" && i+2 < len(parts) {
			return fmt.Sprintf("%s/%s", parts[i+1], parts[i+2])
		}
	}

	// Fallback: use the last directory name or "default"
	if len(parts) > 0 && parts[len(parts)-1] != "" {
		return parts[len(parts)-1]
	}

	return "default"
}

// ExecuteChildWorkflow implements WorkflowRunner interface for fan-out step execution.
func (r *Runner) ExecuteChildWorkflow(ctx context.Context, repoPath, workflowName string, inputs map[string]string) (string, error) {
	// Call the existing ExecuteWorkflow method but return just the run ID
	result, err := r.ExecuteWorkflow(ctx, workflowName, inputs, repoPath)
	if err != nil {
		return "", err
	}

	return result.RunID, nil
}

// Close cleans up the runner resources.
func (r *Runner) Close() error {
	if r.locks != nil {
		return r.locks.Close()
	}
	return nil
}
