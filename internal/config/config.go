package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type TakoConfig struct {
	Version       string              `yaml:"version"`
	Metadata      Metadata            `yaml:"metadata"`
	Artifacts     map[string]Artifact `yaml:"artifacts"`
	Dependents    []Dependent         `yaml:"dependents"` // Legacy field, still required for graph functionality
	Workflows     map[string]Workflow `yaml:"workflows"`
	Subscriptions []Subscription      `yaml:"subscriptions,omitempty"` // New event-driven subscriptions
}

type Metadata struct {
	Name string `yaml:"name"`
}

type Artifact struct {
	Name           string `yaml:"-"`
	Description    string `yaml:"description"`
	Image          string `yaml:"image"`
	Command        string `yaml:"command"`
	Path           string `yaml:"path"`
	Ecosystem      string `yaml:"ecosystem,omitempty"` // New field for artifact ecosystem (go, npm, maven, etc.)
	InstallCommand string `yaml:"install_command"`
	VerifyCommand  string `yaml:"verify_command"`
}

type Dependent struct {
	Repo      string   `yaml:"repo"`
	Artifacts []string `yaml:"artifacts"`
	Workflows []string `yaml:"workflows"`
}

type Workflow struct {
	Name      string                   `yaml:"-"`
	On        string                   `yaml:"on,omitempty"` // Trigger condition ("exec" for manual workflows)
	Image     string                   `yaml:"image,omitempty"`
	Env       []string                 `yaml:"env,omitempty"`
	Secrets   []string                 `yaml:"secrets,omitempty"` // List of secret names
	Resources Resources                `yaml:"resources,omitempty"`
	Inputs    map[string]WorkflowInput `yaml:"inputs,omitempty"` // Input definitions
	Steps     []WorkflowStep           `yaml:"steps,omitempty"`  // Updated to support rich step definitions
}

type Resources struct {
	CPU       string `yaml:"cpu,omitempty"`
	Memory    string `yaml:"memory,omitempty"`
	CPULimit  string `yaml:"cpu_limit,omitempty"`  // New format for resource limits
	MemLimit  string `yaml:"mem_limit,omitempty"`  // New format for memory limits
	DiskLimit string `yaml:"disk_limit,omitempty"` // Disk space limit
}

// WorkflowInput represents an input parameter for a workflow.
type WorkflowInput struct {
	Type        string                  `yaml:"type,omitempty"`        // Input type (string, boolean, number)
	Description string                  `yaml:"description,omitempty"` // Human-readable description
	Required    bool                    `yaml:"required,omitempty"`    // Whether input is required
	Default     interface{}             `yaml:"default,omitempty"`     // Default value
	Validation  WorkflowInputValidation `yaml:"validation,omitempty"`  // Validation rules
}

// WorkflowInputValidation represents validation rules for workflow inputs.
type WorkflowInputValidation struct {
	Enum []string `yaml:"enum,omitempty"` // List of allowed values
	Min  *float64 `yaml:"min,omitempty"`  // Minimum value for numbers
	Max  *float64 `yaml:"max,omitempty"`  // Maximum value for numbers
}

// WorkflowStep represents a single step in a workflow.
type WorkflowStep struct {
	ID            string                 `yaml:"id,omitempty"`              // Step identifier
	If            string                 `yaml:"if,omitempty"`              // Conditional execution (CEL expression)
	Run           string                 `yaml:"run,omitempty"`             // Command to run
	Uses          string                 `yaml:"uses,omitempty"`            // Built-in step to use (e.g., "tako/checkout@v1")
	With          map[string]interface{} `yaml:"with,omitempty"`            // Parameters for built-in steps
	Image         string                 `yaml:"image,omitempty"`           // Container image override
	LongRunning   bool                   `yaml:"long_running,omitempty"`    // Whether step runs asynchronously
	Network       string                 `yaml:"network,omitempty"`         // Network access level
	Capabilities  []string               `yaml:"capabilities,omitempty"`    // Linux capabilities to grant
	CacheKeyFiles string                 `yaml:"cache_key_files,omitempty"` // Glob pattern for cache key
	Env           map[string]string      `yaml:"env,omitempty"`             // Environment variables
	Produces      *WorkflowStepProduces  `yaml:"produces,omitempty"`        // Step outputs and events
	OnFailure     []WorkflowStep         `yaml:"on_failure,omitempty"`      // Steps to run on failure
}

// WorkflowStepProduces represents what a step produces (outputs, artifacts, events).
type WorkflowStepProduces struct {
	Artifact string            `yaml:"artifact,omitempty"` // Artifact name produced
	Outputs  map[string]string `yaml:"outputs,omitempty"`  // Output mappings
	Events   []Event           `yaml:"events,omitempty"`   // Events to emit
}

// UnmarshalYAML implements custom YAML unmarshaling for WorkflowStep to support backward compatibility.
func (step *WorkflowStep) UnmarshalYAML(node *yaml.Node) error {
	// If the node is a string, this is a legacy simple step
	if node.Kind == yaml.ScalarNode {
		step.Run = node.Value
		return nil
	}

	// If the node is a mapping, this is a new-style step
	if node.Kind == yaml.MappingNode {
		// Create a temporary struct to avoid infinite recursion
		type WorkflowStepAlias WorkflowStep
		alias := (*WorkflowStepAlias)(step)
		return node.Decode(alias)
	}

	return fmt.Errorf("step must be either a string or an object")
}

func Load(path string) (*TakoConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read config file: %w", err)
	}

	var config TakoConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("could not unmarshal config: %w", err)
	}

	for name, artifact := range config.Artifacts {
		artifact.Name = name
		config.Artifacts[name] = artifact
	}

	for name, workflow := range config.Workflows {
		workflow.Name = name
		config.Workflows[name] = workflow
	}

	if err := validate(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func validate(config *TakoConfig) error {
	if config.Version == "" {
		return fmt.Errorf("missing required field: version")
	}

	// Validate subscriptions (new event-driven model)
	if len(config.Subscriptions) > 0 {
		if err := ValidateSubscriptions(config.Subscriptions); err != nil {
			return fmt.Errorf("invalid subscriptions: %w", err)
		}

		// Validate that referenced workflows exist
		for i, subscription := range config.Subscriptions {
			if _, exists := config.Workflows[subscription.Workflow]; !exists {
				return fmt.Errorf("subscription %d references non-existent workflow '%s'", i, subscription.Workflow)
			}
		}
	}

	// Validate dependents (legacy model) - only if subscriptions are not present
	if len(config.Subscriptions) == 0 {
		if config.Dependents == nil {
			return fmt.Errorf("missing required field: dependents (or subscriptions for event-driven workflows)")
		}

		for _, dependent := range config.Dependents {
			if err := validateRepoFormat(dependent.Repo); err != nil {
				return err
			}
			if err := validateArtifacts(dependent.Artifacts, config.Artifacts); err != nil {
				return err
			}
		}
	}

	// Validate workflows
	for workflowName, workflow := range config.Workflows {
		if err := validateWorkflow(workflowName, &workflow); err != nil {
			return fmt.Errorf("invalid workflow '%s': %w", workflowName, err)
		}
	}

	return nil
}

func validateRepoFormat(repo string) error {
	// Local paths are not validated
	if strings.HasPrefix(repo, ".") || strings.HasPrefix(repo, "file://") {
		return nil
	}
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format: %s", repo)
	}
	if !strings.Contains(parts[1], ":") {
		return fmt.Errorf("invalid repo format, missing branch: %s", repo)
	}
	return nil
}

func validateArtifacts(dependentArtifacts []string, definedArtifacts map[string]Artifact) error {
	for _, dependentArtifact := range dependentArtifacts {
		if _, ok := definedArtifacts[dependentArtifact]; !ok {
			return fmt.Errorf("dependent artifact not found: %s", dependentArtifact)
		}
	}
	return nil
}

// validateWorkflow validates a single workflow definition.
func validateWorkflow(_ string, workflow *Workflow) error {
	// Validate workflow inputs
	for inputName, input := range workflow.Inputs {
		if err := validateWorkflowInput(inputName, &input); err != nil {
			return fmt.Errorf("invalid input '%s': %w", inputName, err)
		}
	}

	// Validate workflow steps
	for i, step := range workflow.Steps {
		if err := validateWorkflowStep(i, &step); err != nil {
			return fmt.Errorf("invalid step %d: %w", i, err)
		}
	}

	return nil
}

// validateWorkflowInput validates a workflow input definition.
func validateWorkflowInput(_ string, input *WorkflowInput) error {
	// Validate input type
	if input.Type != "" {
		validTypes := []string{"string", "boolean", "number"}
		valid := false
		for _, validType := range validTypes {
			if input.Type == validType {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid input type '%s', must be one of: %v", input.Type, validTypes)
		}
	}

	// Validate enum values if present
	if len(input.Validation.Enum) > 0 && input.Type != "string" && input.Type != "" {
		return fmt.Errorf("enum validation is only supported for string inputs")
	}

	// Validate min/max for numeric inputs
	if (input.Validation.Min != nil || input.Validation.Max != nil) && input.Type != "number" && input.Type != "" {
		return fmt.Errorf("min/max validation is only supported for number inputs")
	}

	return nil
}

// validateWorkflowStep validates a single workflow step.
func validateWorkflowStep(_ int, step *WorkflowStep) error {
	// Either 'run' or 'uses' must be specified, but not both
	if step.Run == "" && step.Uses == "" {
		return fmt.Errorf("step must specify either 'run' or 'uses'")
	}
	if step.Run != "" && step.Uses != "" {
		return fmt.Errorf("step cannot specify both 'run' and 'uses'")
	}

	// Validate built-in step format and known steps
	if step.Uses != "" {
		if err := validateBuiltinStep(step.Uses); err != nil {
			return err
		}
	}

	// Validate produces section if present
	if step.Produces != nil {
		if err := validateWorkflowStepProduces(step.Produces); err != nil {
			return fmt.Errorf("invalid produces section: %w", err)
		}
	}

	// Validate failure steps
	for i, failureStep := range step.OnFailure {
		if err := validateWorkflowStep(i, &failureStep); err != nil {
			return fmt.Errorf("invalid failure step %d: %w", i, err)
		}
	}

	return nil
}

// validateWorkflowStepProduces validates the produces section of a step.
func validateWorkflowStepProduces(produces *WorkflowStepProduces) error {
	// Validate output formats
	for outputName, outputValue := range produces.Outputs {
		if outputValue == "" {
			return fmt.Errorf("output '%s' cannot have empty value", outputName)
		}

		// Validate known output formats
		validFormats := []string{"from_stdout", "from_stderr", "from_file:", "from_env:"}
		valid := false
		for _, format := range validFormats {
			if outputValue == format || strings.HasPrefix(outputValue, format) {
				valid = true
				break
			}
		}
		if !valid && !strings.HasPrefix(outputValue, "{{") {
			return fmt.Errorf("output '%s' has invalid format '%s'", outputName, outputValue)
		}
	}

	// Validate events
	if len(produces.Events) > 0 {
		eventProduction := EventProduction{Events: produces.Events}
		if err := eventProduction.ValidateEvents(); err != nil {
			return fmt.Errorf("invalid events: %w", err)
		}
	}

	return nil
}

// knownBuiltinSteps defines the known built-in steps with their supported versions.
var knownBuiltinSteps = map[string][]string{
	"tako/checkout":            {"v1"},
	"tako/fan-out":             {"v1"},
	"tako/update-dependency":   {"v1"},
	"tako/create-pull-request": {"v1"},
	"tako/poll":                {"v1"},
}

// validateBuiltinStep validates that a built-in step is known and uses a supported version.
func validateBuiltinStep(uses string) error {
	parts := strings.Split(uses, "@")
	if len(parts) != 2 {
		return fmt.Errorf("built-in step '%s' must include version (e.g., 'tako/checkout@v1')", uses)
	}

	stepName := parts[0]
	version := parts[1]

	supportedVersions, exists := knownBuiltinSteps[stepName]
	if !exists {
		return fmt.Errorf("unknown built-in step '%s'", stepName)
	}

	for _, supportedVersion := range supportedVersions {
		if version == supportedVersion {
			return nil
		}
	}

	return fmt.Errorf("built-in step '%s' version '%s' is not supported. Supported versions: %v", stepName, version, supportedVersions)
}

// validateCELExpression validates CEL expression syntax (basic validation).
func validateCELExpression(expression string) error {
	// Basic validation for common CEL patterns
	// This is a simplified validation - in production, you'd use the actual CEL library
	if strings.TrimSpace(expression) == "" {
		return fmt.Errorf("CEL expression cannot be empty")
	}

	// Check for balanced parentheses
	parenCount := 0
	for _, char := range expression {
		switch char {
		case '(':
			parenCount++
		case ')':
			parenCount--
			if parenCount < 0 {
				return fmt.Errorf("unbalanced parentheses in CEL expression: %s", expression)
			}
		}
	}
	if parenCount != 0 {
		return fmt.Errorf("unbalanced parentheses in CEL expression: %s", expression)
	}

	// Basic syntax validation for common patterns
	invalidPatterns := []string{
		"&&&&", "||||", "...", "!!!",
	}
	for _, pattern := range invalidPatterns {
		if strings.Contains(expression, pattern) {
			return fmt.Errorf("invalid pattern '%s' in CEL expression: %s", pattern, expression)
		}
	}

	return nil
}

// validateSemverRange validates semantic version range syntax.
func validateSemverRange(versionRange string) error {
	if versionRange == "" {
		return nil // Empty version range is valid
	}

	// Patterns for semantic version ranges
	patterns := []string{
		`^\^\d+\.\d+\.\d+$`,                         // ^1.0.0
		`^~\d+\.\d+\.\d+$`,                          // ~1.0.0
		`^\d+\.\d+\.\d+$`,                           // 1.0.0
		`^\(\d+\.\d+\.\d+\.\.\.(\d+\.\d+\.\d+)?\]$`, // (1.0.0...2.0.0]
		`^\[\d+\.\d+\.\d+\.\.\.(\d+\.\d+\.\d+)?\)$`, // [1.0.0...2.0.0)
	}

	for _, pattern := range patterns {
		matched, err := regexp.MatchString(pattern, versionRange)
		if err != nil {
			return fmt.Errorf("error validating version range pattern: %w", err)
		}
		if matched {
			return nil
		}
	}

	return fmt.Errorf("invalid version range format '%s'. Supported formats: ^1.0.0, ~1.0.0, 1.0.0, (1.0.0...2.0.0], [1.0.0...2.0.0)", versionRange)
}

// validateTemplateExpression validates template expressions in payload fields.
func validateTemplateExpression(expression string) error {
	if !strings.Contains(expression, "{{") {
		return nil // Not a template expression
	}

	// Count opening and closing braces
	openCount := strings.Count(expression, "{{")
	closeCount := strings.Count(expression, "}}")

	if openCount != closeCount {
		return fmt.Errorf("unbalanced template braces in expression: %s", expression)
	}

	// Basic validation for template content
	if strings.Contains(expression, "{{}}") {
		return fmt.Errorf("empty template expression in: %s", expression)
	}

	return nil
}
