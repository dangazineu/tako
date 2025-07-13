package config

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func buildYAML(version *string, dependents []string) string {
	var sb strings.Builder
	if version != nil {
		sb.WriteString(fmt.Sprintf("version: %q\n", *version))
	}
	if dependents != nil {
		sb.WriteString("dependents:\n")
		for _, d := range dependents {
			sb.WriteString(fmt.Sprintf("  - repo: %q\n", d))
		}
	}
	return sb.String()
}

func stringPtr(s string) *string {
	return &s
}

func TestLoad(t *testing.T) {
	testCases := []struct {
		name        string
		version     *string
		dependents  []string
		extra       string
		expectError bool
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpfile, err := os.CreateTemp(t.TempDir(), "tako.yml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())

			content := buildYAML(tc.version, tc.dependents) + tc.extra
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
