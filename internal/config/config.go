package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version       string              `yaml:"version"`
	Artifacts     map[string]Artifact `yaml:"artifacts"`
	Dependents    []Dependent         `yaml:"dependents,omitempty"`
	Workflows     map[string]Workflow `yaml:"workflows"`
	Subscriptions []Subscription      `yaml:"subscriptions,omitempty"`
}

type Artifact struct {
	Name      string `yaml:"-"`
	Path      string `yaml:"path"`
	Ecosystem string `yaml:"ecosystem,omitempty"`
}

type Dependent struct {
	Repo      string   `yaml:"repo"`
	Artifacts []string `yaml:"artifacts"`
	Workflows []string `yaml:"workflows"`
}

type Workflow struct {
	Name      string                   `yaml:"-"`
	On        string                   `yaml:"on,omitempty"`
	Image     string                   `yaml:"image,omitempty"`
	Env       []string                 `yaml:"env,omitempty"`
	Secrets   []string                 `yaml:"secrets,omitempty"`
	Resources Resources                `yaml:"resources,omitempty"`
	Inputs    map[string]WorkflowInput `yaml:"inputs,omitempty"`
	Steps     []WorkflowStep           `yaml:"steps,omitempty"`
}

type Resources struct {
	CPULimit  string `yaml:"cpu_limit,omitempty"`
	MemLimit  string `yaml:"mem_limit,omitempty"`
	DiskLimit string `yaml:"disk_limit,omitempty"`
}

type WorkflowInput struct {
	Type        string                  `yaml:"type,omitempty"`
	Description string                  `yaml:"description,omitempty"`
	Required    bool                    `yaml:"required,omitempty"`
	Default     interface{}             `yaml:"default,omitempty"`
	Validation  WorkflowInputValidation `yaml:"validation,omitempty"`
}

type WorkflowInputValidation struct {
	Enum []string `yaml:"enum,omitempty"`
	Min  *float64 `yaml:"min,omitempty"`
	Max  *float64 `yaml:"max,omitempty"`
}

type WorkflowStep struct {
	ID              string                 `yaml:"id,omitempty"`
	If              string                 `yaml:"if,omitempty"`
	Run             string                 `yaml:"run,omitempty"`
	Uses            string                 `yaml:"uses,omitempty"`
	With            map[string]interface{} `yaml:"with,omitempty"`
	Image           string                 `yaml:"image,omitempty"`
	LongRunning     bool                   `yaml:"long_running,omitempty"`
	Network         string                 `yaml:"network,omitempty"`
	Capabilities    []string               `yaml:"capabilities,omitempty"`
	SecurityProfile string                 `yaml:"security_profile,omitempty"`
	Volumes         []VolumeMount          `yaml:"volumes,omitempty"`
	CacheKeyFiles   string                 `yaml:"cache_key_files,omitempty"`
	Env             map[string]string      `yaml:"env,omitempty"`
	Resources       *Resources             `yaml:"resources,omitempty"`
	Produces        *WorkflowStepProduces  `yaml:"produces,omitempty"`
	OnFailure       []WorkflowStep         `yaml:"on_failure,omitempty"`
}

// VolumeMount represents a volume mount for containerized steps.
type VolumeMount struct {
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
	ReadOnly    bool   `yaml:"read_only,omitempty"`
}

type WorkflowStepProduces struct {
	Artifact string            `yaml:"artifact,omitempty"`
	Outputs  map[string]string `yaml:"outputs,omitempty"`
	Events   []Event           `yaml:"events,omitempty"`
}

func (step *WorkflowStep) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		step.Run = node.Value
		return nil
	}

	if node.Kind == yaml.MappingNode {
		type WorkflowStepAlias WorkflowStep
		alias := (*WorkflowStepAlias)(step)
		return node.Decode(alias)
	}

	return fmt.Errorf("step must be either a string or an object")
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("could not unmarshal config: %w", err)
	}

	for name := range config.Artifacts {
		artifact := config.Artifacts[name]
		artifact.Name = name
		config.Artifacts[name] = artifact
	}

	for name := range config.Workflows {
		workflow := config.Workflows[name]
		workflow.Name = name
		config.Workflows[name] = workflow
	}

	if err := validate(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func validate(config *Config) error {
	if config.Version == "" {
		return fmt.Errorf("missing required field: version")
	}

	if len(config.Subscriptions) > 0 {
		if err := ValidateSubscriptions(config.Subscriptions); err != nil {
			return fmt.Errorf("invalid subscriptions: %w", err)
		}

		for i, subscription := range config.Subscriptions {
			if _, exists := config.Workflows[subscription.Workflow]; !exists {
				return fmt.Errorf("subscription %d references non-existent workflow '%s'", i, subscription.Workflow)
			}
		}
	}

	// Validate dependents
	for _, dependent := range config.Dependents {
		if err := validateRepoFormat(dependent.Repo); err != nil {
			return fmt.Errorf("invalid dependent repo format: %w", err)
		}
		if err := validateArtifacts(dependent.Artifacts, config.Artifacts); err != nil {
			return fmt.Errorf("invalid dependent artifacts: %w", err)
		}
	}

	for workflowName, workflow := range config.Workflows {
		if err := validateWorkflow(workflowName, &workflow); err != nil {
			return fmt.Errorf("invalid workflow '%s': %w", workflowName, err)
		}
	}

	return nil
}

func validateWorkflow(_ string, workflow *Workflow) error {
	for inputName, input := range workflow.Inputs {
		if err := validateWorkflowInput(inputName, &input); err != nil {
			return fmt.Errorf("invalid input '%s': %w", inputName, err)
		}
	}

	for i, step := range workflow.Steps {
		if err := validateWorkflowStep(i, &step); err != nil {
			return fmt.Errorf("invalid step %d: %w", i, err)
		}
	}

	return nil
}

func validateWorkflowInput(_ string, input *WorkflowInput) error {
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

	if len(input.Validation.Enum) > 0 && input.Type != "string" && input.Type != "" {
		return fmt.Errorf("enum validation is only supported for string inputs")
	}

	if (input.Validation.Min != nil || input.Validation.Max != nil) && input.Type != "number" && input.Type != "" {
		return fmt.Errorf("min/max validation is only supported for number inputs")
	}

	return nil
}

func validateWorkflowStep(_ int, step *WorkflowStep) error {
	if step.Run == "" && step.Uses == "" {
		return fmt.Errorf("step must specify either 'run' or 'uses'")
	}
	if step.Run != "" && step.Uses != "" {
		return fmt.Errorf("step cannot specify both 'run' and 'uses'")
	}

	if step.Uses != "" {
		if err := validateBuiltinStep(step.Uses); err != nil {
			return err
		}
	}

	if step.Produces != nil {
		if err := validateWorkflowStepProduces(step.Produces); err != nil {
			return fmt.Errorf("invalid produces section: %w", err)
		}
	}

	for i, failureStep := range step.OnFailure {
		if err := validateWorkflowStep(i, &failureStep); err != nil {
			return fmt.Errorf("invalid failure step %d: %w", i, err)
		}
	}

	return nil
}

func validateWorkflowStepProduces(produces *WorkflowStepProduces) error {
	for outputName, outputValue := range produces.Outputs {
		if outputValue == "" {
			return fmt.Errorf("output '%s' cannot have empty value", outputName)
		}

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

	if len(produces.Events) > 0 {
		eventProduction := EventProduction{Events: produces.Events}
		if err := eventProduction.ValidateEvents(); err != nil {
			return fmt.Errorf("invalid events: %w", err)
		}
	}

	return nil
}

var knownBuiltinSteps = map[string][]string{
	"tako/checkout":            {"v1"},
	"tako/fan-out":             {"v1"},
	"tako/update-dependency":   {"v1"},
	"tako/create-pull-request": {"v1"},
	"tako/poll":                {"v1"},
}

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

func validateCELExpression(expression string) error {
	if strings.TrimSpace(expression) == "" {
		return fmt.Errorf("CEL expression cannot be empty")
	}

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

func validateSemverRange(versionRange string) error {
	if versionRange == "" {
		return nil
	}

	patterns := []string{
		`^\^\d+\.\d+\.\d+$`,
		`^~\d+\.\d+\.\d+$`,
		`^\d+\.\d+\.\d+$`,
		`^\(\d+\.\d+\.\d+\.\.\.(\d+\.\d+\.\d+)?\]$`,
		`^\[\d+\.\d+\.\d+\.\.\.(\d+\.\d+\.\d+)?\)$`,
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

func validateTemplateExpression(expression string) error {
	if !strings.Contains(expression, "{{") {
		return nil
	}

	openCount := strings.Count(expression, "{{")
	closeCount := strings.Count(expression, "}}")

	if openCount != closeCount {
		return fmt.Errorf("unbalanced template braces in expression: %s", expression)
	}

	if strings.Contains(expression, "{{}}") {
		return fmt.Errorf("empty template expression in: %s", expression)
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
