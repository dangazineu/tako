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
	"text/template"
	"time"

	"github.com/dangazineu/tako/internal/config"
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

	// Configuration
	maxConcurrentRepos int
	dryRun             bool
	debug              bool
	noCache            bool
	environment        []string

	// Synchronization
	mu sync.RWMutex
}

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
	// TODO: Implement multi-repository execution with event-driven orchestration
	// This will be the main orchestration logic that handles:
	// 1. Parent repository workflow execution
	// 2. Event emission via tako/fan-out@v1 steps
	// 3. Child repository subscription evaluation
	// 4. Parallel execution of child workflows
	// 5. State synchronization across all repositories

	return nil, fmt.Errorf("multi-repository execution not yet implemented")
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
		return r.executeBuiltinStep(step, stepID, startTime)
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
func (r *Runner) executeBuiltinStep(step config.WorkflowStep, stepID string, startTime time.Time) (StepResult, error) {
	// TODO: Implement built-in steps like tako/fan-out@v1
	// For now, return not implemented

	err := fmt.Errorf("built-in steps not yet implemented: %s", step.Uses)
	r.state.FailStep(stepID, err.Error())

	return StepResult{
		ID:        stepID,
		Success:   false,
		Error:     err,
		StartTime: startTime,
		EndTime:   time.Now(),
	}, err
}

// expandTemplate expands template variables in a string.
func (r *Runner) expandTemplate(tmplStr string, inputs map[string]string, stepOutputs map[string]map[string]string) (string, error) {
	// Create template data structure
	data := map[string]interface{}{
		"inputs": inputs,
		"steps":  stepOutputs,
	}

	// Parse and execute template
	tmpl, err := template.New("step").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	return buf.String(), nil
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

// Close cleans up the runner resources.
func (r *Runner) Close() error {
	if r.locks != nil {
		return r.locks.Close()
	}
	return nil
}
