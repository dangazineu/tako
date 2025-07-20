package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type TakoConfig struct {
	Version    string              `yaml:"version"`
	Metadata   Metadata            `yaml:"metadata"`
	Artifacts  map[string]Artifact `yaml:"artifacts"`
	Dependents []Dependent         `yaml:"dependents"`
	Workflows  map[string]Workflow `yaml:"workflows"`
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
	InstallCommand string `yaml:"install_command"`
	VerifyCommand  string `yaml:"verify_command"`
}

type Dependent struct {
	Repo      string   `yaml:"repo"`
	Artifacts []string `yaml:"artifacts"`
	Workflows []string `yaml:"workflows"`
}

type Workflow struct {
	Name      string    `yaml:"-"`
	Image     string    `yaml:"image"`
	Env       []string  `yaml:"env"`
	Resources Resources `yaml:"resources"`
	Steps     []string  `yaml:"steps"`
}

type Resources struct {
	CPU    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
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

	if config.Dependents == nil {
		return fmt.Errorf("missing required field: dependents")
	}

	for _, dependent := range config.Dependents {
		if err := validateRepoFormat(dependent.Repo); err != nil {
			return err
		}
		if err := validateArtifacts(dependent.Artifacts, config.Artifacts); err != nil {
			return err
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
