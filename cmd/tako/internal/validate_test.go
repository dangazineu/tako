package internal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCmd(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a mock tako.yml file
	takoYml := `
version: 0.1.0
metadata:
  name: repo-a
dependents: []
`
	takoPath := filepath.Join(tmpDir, "tako.yml")
	err := os.WriteFile(takoPath, []byte(takoYml), 0644)
	if err != nil {
		t.Fatalf("failed to write tako.yml: %v", err)
	}

	// Execute the validate command
	b := bytes.NewBufferString("")
	cmd := NewRootCmd()
	cmd.SetOut(b)
	cmd.SetArgs([]string{"validate", "--root", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute validate command: %v", err)
	}

	// Check the output
	expected := "Validation successful!"
	if !strings.Contains(b.String(), expected) {
		t.Errorf("expected output to contain %q, got %q", expected, b.String())
	}
}