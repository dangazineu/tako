package config

import (
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

type TakoConfig struct {
	Version    string      `yaml:"version"`
	Metadata   Metadata    `yaml:"metadata"`
	Artifacts  []Artifact  `yaml:"artifacts"`
	Dependents []Dependent `yaml:"dependents"`
	Workflows  []Workflow  `yaml:"workflows"`
}

type Metadata struct {
	Name string `yaml:"name"`
}

type Artifact struct {
	Name           string `yaml:"name"`
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
	Name      string    `yaml:"name"`
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

func validateArtifacts(dependentArtifacts []string, definedArtifacts []Artifact) error {
	for _, dependentArtifact := range dependentArtifacts {
		found := false
		for _, definedArtifact := range definedArtifacts {
			if dependentArtifact == definedArtifact.Name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("dependent artifact not found: %s", dependentArtifact)
		}
	}
	return nil
}
