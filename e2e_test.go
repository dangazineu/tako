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

func TestE2E(t *testing.T) {
	for name, tc := range e2e.TestCases {
		tc := tc // capture range variable
		t.Run(name, func(t *testing.T) {
			t.Run("local", func(t *testing.T) {
				runTest(t, &tc, "local")
			})
			t.Run("remote", func(t *testing.T) {
				if testing.Short() {
					t.Skip("skipping remote test in short mode")
				}
				runTest(t, &tc, "remote")
			})
		})
	}
}

func runTest(t *testing.T, tc *e2e.TestCase, mode string) {
	var testCaseDir string
	var err error

	if mode == "local" {
		testCaseDir, err = tc.SetupLocal()
		if err != nil {
			t.Fatalf("failed to setup local test case: %v", err)
		}
	} else {
		// Clear cache before remote tests to avoid inconsistent state
		homeDir, err := os.UserHomeDir()
		if err == nil {
			os.RemoveAll(filepath.Join(homeDir, ".tako", "cache", "repos", "tako-test"))
		}

		client, err := e2e.GetClient()
		if err != nil {
			t.Fatalf("failed to get github client: %v", err)
		}
		if err := tc.Setup(client); err != nil {
			t.Fatalf("failed to setup remote test case: %v", err)
		}
		t.Cleanup(func() {
			if err := tc.Cleanup(client); err != nil {
				t.Errorf("failed to cleanup remote test case: %v", err)
			}
		})
		tmpDir := t.TempDir()
		cmd := exec.Command("git", "clone", tc.Repositories[0].CloneURL, tmpDir)
		err = cmd.Run()
		if err != nil {
			t.Fatalf("failed to clone repo: %v", err)
		}
		testCaseDir = tmpDir
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
	var rootPath string
	if mode == "local" {
		// For local mode, testCaseDir contains subdirectories for each repo
		rootPath = filepath.Join(testCaseDir, tc.Repositories[0].Name)
	} else {
		// For remote mode, testCaseDir is the cloned repository root
		rootPath = testCaseDir
	}
	takoCmd := exec.Command(takoPath, "graph", "--root", rootPath)
	takoCmd.Stdout = &out
	takoCmd.Stderr = &out
	err = takoCmd.Run()
	if err != nil {
		t.Fatalf("failed to run tako graph: %v\nOutput:\n%s", err, out.String())
	}

	// Assert the output
	expected := getExpectedOutput(tc.Name)
	if testing.Verbose() {
		t.Logf("Expected output:\n%s", expected)
		t.Logf("Actual output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), expected) {
		t.Errorf("expected output to contain %q, got %q", expected, out.String())
	}
}

func getExpectedOutput(testCaseName string) string {
	switch testCaseName {
	case "deep-graph":
		return `repo-x
└── repo-y
    └── repo-z
`
	case "diamond-dependency-graph":
		return `repo-a
├── repo-b
│   └── repo-c
│       └── repo-e
└── repo-d
    └── repo-e
`
	default:
		return ""
	}
}
