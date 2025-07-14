//go:build e2e
// +build e2e

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dangazineu/tako/test/e2e"
)

func findProjectRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// TestE2E_SimpleGraph tests a simple dependency graph: repo-a -> repo-b.
// It creates a temporary directory with two subdirectories, repo-a and repo-b.
// Each subdirectory contains a tako.yml file.
// repo-a/tako.yml defines a dependency on repo-b.
// The test then runs `tako graph` from the repo-a directory and asserts that the
// output is correct.
func TestE2E_SimpleGraph(t *testing.T) {
	// Get the test case
	testCase, ok := e2e.TestCases["simple-graph"]
	if !ok {
		t.Fatal("test case not found")
	}

	// Create a temporary directory for the test
	tmpDir := t.TempDir()
	if err := testCase.SetupLocal(tmpDir); err != nil {
		t.Fatalf("failed to setup local test case: %v", err)
	}

	// Build the tako binary
	takoPath := filepath.Join(t.TempDir(), "tako")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	projectRoot := findProjectRoot(wd)
	if projectRoot == "" {
		t.Fatal("failed to find project root")
	}
	buildCmd := exec.Command("go", "build", "-o", takoPath, "./cmd/tako")
	buildCmd.Dir = projectRoot
	var buildOut bytes.Buffer
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut
	err = buildCmd.Run()
	if err != nil {
		t.Fatalf("failed to build tako binary: %v\nOutput:\n%s", err, buildOut.String())
	}

	// Run tako graph
	var out bytes.Buffer
	takoCmd := exec.Command(takoPath, "graph", "--root", filepath.Join(tmpDir, "repo-a"))
	takoCmd.Stdout = &out
	takoCmd.Stderr = &out
	err = takoCmd.Run()
	if err != nil {
		t.Fatalf("failed to run tako graph: %v\nOutput:\n%s", err, out.String())
	}

	// Assert the output
	expected := `repo-a
└── repo-b
`
	if testing.Verbose() {
		t.Logf("Expected output:\n%s", expected)
		t.Logf("Actual output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), expected) {
		t.Errorf("expected output to contain %q, got %q", expected, out.String())
	}
}

// TestE2E_ComplexGraph tests a more complex dependency graph with a diamond dependency:
// repo-a -> repo-b -> repo-c -> repo-e
// repo-a -> repo-d -> repo-e
// It creates a temporary directory with five subdirectories, repo-a, repo-b, repo-c, repo-d, and repo-e.
// Each subdirectory contains a tako.yml file with the corresponding dependencies.
// The test then runs `tako graph` from the repo-a directory and asserts that the
// output is correct.
func TestE2E_ComplexGraph(t *testing.T) {
	// Get the test case
	testCase, ok := e2e.TestCases["complex-graph"]
	if !ok {
		t.Fatal("test case not found")
	}

	// Create a temporary directory for the test
	tmpDir := t.TempDir()
	if err := testCase.SetupLocal(tmpDir); err != nil {
		t.Fatalf("failed to setup local test case: %v", err)
	}

	// Build the tako binary
	takoPath := filepath.Join(t.TempDir(), "tako")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	projectRoot := findProjectRoot(wd)
	if projectRoot == "" {
		t.Fatal("failed to find project root")
	}
	buildCmd := exec.Command("go", "build", "-o", takoPath, "./cmd/tako")
	buildCmd.Dir = projectRoot
	var buildOut bytes.Buffer
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut
	err = buildCmd.Run()
	if err != nil {
		t.Fatalf("failed to build tako binary: %v\nOutput:\n%s", err, buildOut.String())
	}

	// Run tako graph
	var out bytes.Buffer
	takoCmd := exec.Command(takoPath, "graph", "--root", filepath.Join(tmpDir, "repo-a"))
	takoCmd.Stdout = &out
	takoCmd.Stderr = &out
	err = takoCmd.Run()
	if err != nil {
		t.Fatalf("failed to run tako graph: %v\nOutput:\n%s", err, out.String())
	}

	// Assert the output
	expected := `repo-a
├── repo-b
│   └── repo-c
│       └── repo-e
└── repo-d
    └── repo-e
`
	if testing.Verbose() {
		t.Logf("Expected output:\n%s", expected)
		t.Logf("Actual output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), expected) {
		t.Errorf("expected output to contain %q, got %q", expected, out.String())
	}
}
