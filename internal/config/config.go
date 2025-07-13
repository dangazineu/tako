package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
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

	if config.Version == "" {
		return nil, fmt.Errorf("missing required field: version")
	}

	if config.Dependents == nil {
		return nil, fmt.Errorf("missing required field: dependents")
	}

	return &config, nil
}
