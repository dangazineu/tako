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