package internal

import (
	"bytes"
	"os"
	"os/exec"
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

	// Init git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoA
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}
	cmd = exec.Command("git", "remote", "add", "origin", "https://github.com/test/repo-a.git")
	cmd.Dir = repoA
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add remote: %v", err)
	}

	takoA := `
version: 0.1.0
`
	err := os.WriteFile(filepath.Join(repoA, "tako.yml"), []byte(takoA), 0644)
	if err != nil {
		t.Fatalf("failed to write tako.yml: %v", err)
	}

	// Execute the graph command
	b := bytes.NewBufferString("")
	rootCmd := NewRootCmd()
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"graph", "--root", repoA})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("failed to execute graph command: %v", err)
	}

	// Check the output
	expected := "test/repo-a"
	if !strings.Contains(b.String(), expected) {
		t.Errorf("expected output to contain %q, got %q", expected, b.String())
	}
}
