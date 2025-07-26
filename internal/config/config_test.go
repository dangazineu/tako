package config

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func buildYAML(version *string, dependents []string, artifacts map[string]struct{}, dependentArtifacts []string) string {
	var sb strings.Builder
	if version != nil {
		sb.WriteString(fmt.Sprintf("version: %q\n", *version))
	}
	if artifacts != nil {
		sb.WriteString("artifacts:\n")
		for a := range artifacts {
			sb.WriteString(fmt.Sprintf("  %s:\n    description: %s\n", a, a))
		}
	}
	if dependents != nil {
		sb.WriteString("dependents:\n")
		for i, d := range dependents {
			sb.WriteString(fmt.Sprintf("  - repo: %q\n", d))
			if dependentArtifacts != nil && i < len(dependentArtifacts) {
				sb.WriteString(fmt.Sprintf("    artifacts: [%q]\n", dependentArtifacts[i]))
			}
		}
	}
	return sb.String()
}

func stringPtr(s string) *string {
	return &s
}

func TestLoad(t *testing.T) {
	testCases := []struct {
		name               string
		version            *string
		dependents         []string
		artifacts          map[string]struct{}
		dependentArtifacts []string
		extra              string
		expectError        bool
	}{
		{
			name:        "valid config",
			version:     stringPtr("1.2"),
			dependents:  []string{"my-org/client-a:main"},
			expectError: false,
		},
		{
			name:        "missing version",
			version:     nil,
			dependents:  []string{"my-org/client-a:main"},
			expectError: true,
		},
		{
			name:        "missing dependents",
			version:     stringPtr("1.2"),
			dependents:  nil,
			expectError: true,
		},
		{
			name:        "invalid yaml",
			version:     stringPtr("1.2"),
			dependents:  []string{"my-org/client-a:main"},
			extra:       "  - invalid",
			expectError: true,
		},
		{
			name:        "invalid repo format",
			version:     stringPtr("1.2"),
			dependents:  []string{"my-org-client-a:main"},
			expectError: true,
		},
		{
			name:        "invalid repo format no branch",
			version:     stringPtr("1.2"),
			dependents:  []string{"my-org/client-a"},
			expectError: true,
		},
		{
			name:               "dependent artifact not found",
			version:            stringPtr("1.2"),
			dependents:         []string{"my-org/client-a:main"},
			artifacts:          map[string]struct{}{"art-a": {}},
			dependentArtifacts: []string{"art-b"},
			expectError:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpfile, err := os.CreateTemp(t.TempDir(), "tako.yml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())

			content := buildYAML(tc.version, tc.dependents, tc.artifacts, tc.dependentArtifacts) + tc.extra
			if _, err := tmpfile.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
			if err := tmpfile.Close(); err != nil {
				t.Fatal(err)
			}

			_, err = Load(tmpfile.Name())
			if tc.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoad_PopulatesName(t *testing.T) {
	yamlContent := `
version: "1.0"
artifacts:
  my-artifact:
    description: "An artifact"
workflows:
  my-workflow:
    steps:
      - "echo hello"
dependents: []
`
	tmpfile, err := os.CreateTemp(t.TempDir(), "tako.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	config, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Artifacts["my-artifact"].Name != "my-artifact" {
		t.Errorf("expected artifact name to be 'my-artifact', got %q", config.Artifacts["my-artifact"].Name)
	}

	if config.Workflows["my-workflow"].Name != "my-workflow" {
		t.Errorf("expected workflow name to be 'my-workflow', got %q", config.Workflows["my-workflow"].Name)
	}
}

func TestLoad_EventDrivenWorkflows(t *testing.T) {
	yamlContent := `
version: "0.1.0"
artifacts:
  go-lib:
    path: "./go.mod"
    ecosystem: "go"

workflows:
  release:
    on: "exec"
    inputs:
      version-bump:
        type: "string"
        default: "patch"
        validation:
          enum: ["major", "minor", "patch"]
    steps:
      - id: "build"
        run: "./scripts/build.sh --bump {{ .inputs.version-bump }}"
        produces:
          artifact: "go-lib"
          outputs:
            version: "from_stdout"
          events:
            - type: "library_built"
              schema_version: "1.0.0"
              payload:
                version: "{{ .outputs.version }}"
                commit_sha: "{{ .env.GITHUB_SHA }}"
      - uses: "tako/fan-out@v1"
        with:
          event_type: "library_built"
          wait_for_children: true

subscriptions:
  - artifact: "my-org/go-lib:go-lib"
    events: ["library_built"]
    schema_version: "^1.0.0"
    filters:
      - "semver.major(event.payload.version) > 0"
    workflow: "release"
    inputs:
      upstream_version: "{{ .event.payload.version }}"
`

	tmpfile, err := os.CreateTemp(t.TempDir(), "tako.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	config, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test artifacts
	if len(config.Artifacts) != 1 {
		t.Errorf("expected 1 artifact, got %d", len(config.Artifacts))
	}
	if config.Artifacts["go-lib"].Ecosystem != "go" {
		t.Errorf("expected ecosystem 'go', got %q", config.Artifacts["go-lib"].Ecosystem)
	}

	// Test workflows
	if len(config.Workflows) != 1 {
		t.Errorf("expected 1 workflow, got %d", len(config.Workflows))
	}
	
	releaseWorkflow := config.Workflows["release"]
	if releaseWorkflow.On != "exec" {
		t.Errorf("expected on 'exec', got %q", releaseWorkflow.On)
	}

	// Test inputs
	if len(releaseWorkflow.Inputs) != 1 {
		t.Errorf("expected 1 input, got %d", len(releaseWorkflow.Inputs))
	}
	
	versionBumpInput := releaseWorkflow.Inputs["version-bump"]
	if versionBumpInput.Type != "string" {
		t.Errorf("expected input type 'string', got %q", versionBumpInput.Type)
	}
	if len(versionBumpInput.Validation.Enum) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(versionBumpInput.Validation.Enum))
	}

	// Test steps
	if len(releaseWorkflow.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(releaseWorkflow.Steps))
	}

	buildStep := releaseWorkflow.Steps[0]
	if buildStep.ID != "build" {
		t.Errorf("expected step ID 'build', got %q", buildStep.ID)
	}
	if buildStep.Produces == nil {
		t.Fatal("expected produces section")
	}
	if len(buildStep.Produces.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(buildStep.Produces.Events))
	}

	fanOutStep := releaseWorkflow.Steps[1]
	if fanOutStep.Uses != "tako/fan-out@v1" {
		t.Errorf("expected uses 'tako/fan-out@v1', got %q", fanOutStep.Uses)
	}

	// Test subscriptions
	if len(config.Subscriptions) != 1 {
		t.Errorf("expected 1 subscription, got %d", len(config.Subscriptions))
	}
	
	subscription := config.Subscriptions[0]
	if subscription.Artifact != "my-org/go-lib:go-lib" {
		t.Errorf("expected artifact 'my-org/go-lib:go-lib', got %q", subscription.Artifact)
	}
	if len(subscription.Events) != 1 || subscription.Events[0] != "library_built" {
		t.Errorf("expected events ['library_built'], got %v", subscription.Events)
	}
}

func TestLoad_MixedStepFormats(t *testing.T) {
	yamlContent := `
version: "0.1.0"
workflows:
  mixed-workflow:
    steps:
      - "echo simple step"
      - id: "complex-step"
        run: "echo complex step"
        produces:
          outputs:
            result: "from_stdout"
dependents: []
`

	tmpfile, err := os.CreateTemp(t.TempDir(), "tako.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	config, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	workflow := config.Workflows["mixed-workflow"]
	if len(workflow.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(workflow.Steps))
	}

	// First step should be simple string
	simpleStep := workflow.Steps[0]
	if simpleStep.Run != "echo simple step" {
		t.Errorf("expected run 'echo simple step', got %q", simpleStep.Run)
	}
	if simpleStep.ID != "" {
		t.Errorf("expected empty ID for simple step, got %q", simpleStep.ID)
	}

	// Second step should be complex object
	complexStep := workflow.Steps[1]
	if complexStep.ID != "complex-step" {
		t.Errorf("expected ID 'complex-step', got %q", complexStep.ID)
	}
	if complexStep.Run != "echo complex step" {
		t.Errorf("expected run 'echo complex step', got %q", complexStep.Run)
	}
	if complexStep.Produces == nil || len(complexStep.Produces.Outputs) != 1 {
		t.Error("expected produces section with 1 output")
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name         string
		yamlContent  string
		expectedError string
	}{
		{
			name: "invalid event type",
			yamlContent: `
version: "0.1.0"
workflows:
  test:
    steps:
      - id: "test"
        run: "echo test"
        produces:
          events:
            - type: "invalid-event-type"
dependents: []
`,
			expectedError: "event type 'invalid-event-type' must be snake_case",
		},
		{
			name: "invalid subscription artifact format",
			yamlContent: `
version: "0.1.0"
workflows:
  test:
    steps:
      - "echo test"
subscriptions:
  - artifact: "invalid-format"
    events: ["test_event"]
    workflow: "test"
`,
			expectedError: "artifact reference 'invalid-format' must be in format 'repo:artifact'",
		},
		{
			name: "subscription references non-existent workflow",
			yamlContent: `
version: "0.1.0"
workflows:
  test:
    steps:
      - "echo test"
subscriptions:
  - artifact: "org/repo:artifact"
    events: ["test_event"]
    workflow: "non_existent"
`,
			expectedError: "subscription 0 references non-existent workflow 'non_existent'",
		},
		{
			name: "step with both run and uses",
			yamlContent: `
version: "0.1.0"
workflows:
  test:
    steps:
      - id: "invalid"
        run: "echo test"
        uses: "tako/checkout@v1"
dependents: []
`,
			expectedError: "step cannot specify both 'run' and 'uses'",
		},
		{
			name: "built-in step without version",
			yamlContent: `
version: "0.1.0"
workflows:
  test:
    steps:
      - uses: "tako/checkout"
dependents: []
`,
			expectedError: "built-in step 'tako/checkout' must include version",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpfile, err := os.CreateTemp(t.TempDir(), "tako.yml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())

			if _, err := tmpfile.Write([]byte(tc.yamlContent)); err != nil {
				t.Fatal(err)
			}
			if err := tmpfile.Close(); err != nil {
				t.Fatal(err)
			}

			_, err = Load(tmpfile.Name())
			if err == nil {
				t.Errorf("expected error, got nil")
			} else if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("expected error containing %q, got %q", tc.expectedError, err.Error())
			}
		})
	}
}
