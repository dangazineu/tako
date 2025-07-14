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

func TestE2E_DeepGraph(t *testing.T) {
	// Get the test case
	testCase, ok := e2e.TestCases["deep-graph"]
	if !ok {
		t.Fatal("test case not found")
	}

	// Setup the test case
	testCaseDir, err := testCase.SetupLocal()
	if err != nil {
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
	takoCmd := exec.Command(takoPath, "graph", "--root", filepath.Join(testCaseDir, "repo-x"))
	takoCmd.Stdout = &out
	takoCmd.Stderr = &out
	err = takoCmd.Run()
	if err != nil {
		t.Fatalf("failed to run tako graph: %v\nOutput:\n%s", err, out.String())
	}

	// Assert the output
	expected := `repo-x
└── repo-y
    └── repo-z
`
	if testing.Verbose() {
		t.Logf("Expected output:\n%s", expected)
		t.Logf("Actual output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), expected) {
		t.Errorf("expected output to contain %q, got %q", expected, out.String())
	}
}
