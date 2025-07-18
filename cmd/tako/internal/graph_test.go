package internal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGraphCmd(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a mock repository
	repoA := filepath.Join(tmpDir, "repo-a")
	if err := os.Mkdir(repoA, 0755); err != nil {
		t.Fatalf("failed to create repoA: %v", err)
	}
	takoA := `
version: 0.1.0
metadata:
  name: repo-a
dependents: []
`
	err := os.WriteFile(filepath.Join(repoA, "tako.yml"), []byte(takoA), 0644)
	if err != nil {
		t.Fatalf("failed to write tako.yml: %v", err)
	}

	// Execute the graph command
	b := bytes.NewBufferString("")
	cmd := NewRootCmd()
	cmd.SetOut(b)
	cmd.SetArgs([]string{"graph", "--root", repoA})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute graph command: %v", err)
	}

	// Check the output
	expected := "repo-a"
	if !strings.Contains(b.String(), expected) {
		t.Errorf("expected output to contain %q, got %q", expected, b.String())
	}
}
