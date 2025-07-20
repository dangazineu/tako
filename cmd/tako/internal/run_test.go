package internal_test

import (
	"bytes"
	"github.com/dangazineu/tako/cmd/tako/internal"
	"os"
	"path/filepath"
	"testing"
)

func TestRunCmd(t *testing.T) {
	tmpDir := t.TempDir()
	mustWriteFile(t, filepath.Join(tmpDir, "tako.yml"), `
version: 0.1.0
metadata:
  name: test-repo
`)

	cmd := internal.NewRunCmd()
	b := bytes.NewBufferString("")
	cmd.SetOut(b)
	cmd.SetArgs([]string{"--root", tmpDir, "echo", "hello"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmd_ExecutesOnAllNodes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock repositories
	repoA := filepath.Join(tmpDir, "repo-a")
	repoB := filepath.Join(tmpDir, "repo-b")
	repoC := filepath.Join(tmpDir, "repo-c")
	mustMkdir(t, repoA)
	mustMkdir(t, repoB)
	mustMkdir(t, repoC)

	// Create mock tako.yml files to define the dependency chain: a -> b -> c
	mustWriteFile(t, filepath.Join(repoA, "tako.yml"), `
version: 0.1.0
metadata:
  name: repo-a
dependents:
  - repo: ../repo-b:main
`)
	mustWriteFile(t, filepath.Join(repoB, "tako.yml"), `
version: 0.1.0
metadata:
  name: repo-b
dependents:
  - repo: ../repo-c:main
`)
	mustWriteFile(t, filepath.Join(repoC, "tako.yml"), `
version: 0.1.0
metadata:
  name: repo-c
dependents: []
`)

	// Execute the run command starting from the root of the dependency chain (repo-a)
	cmd := internal.NewRunCmd()
	cmd.SetArgs([]string{"--root", repoA, "--local", "touch", "executed.txt"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run command failed: %v", err)
	}

	// Assert that the command was executed in all repos in the chain
	for _, repoPath := range []string{repoA, repoB, repoC} {
		if _, err := os.Stat(filepath.Join(repoPath, "executed.txt")); os.IsNotExist(err) {
			t.Errorf("command was not executed in %s", filepath.Base(repoPath))
		}
	}
}

func mustMkdir(t *testing.T, path string) {
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatalf("failed to create directory %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}
