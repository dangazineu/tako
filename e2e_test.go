//go:build e2e
// +build e2e

package main

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dangazineu/tako/test/e2e"
)

var (
	local  = flag.Bool("local", false, "run local tests")
	remote = flag.Bool("remote", false, "run remote tests")
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
	if !*local && !*remote {
		t.Fatal("either -local or -remote must be set")
	}
	for name, tc := range e2e.TestCases {
		tc := tc // capture range variable
		t.Run(name, func(t *testing.T) {
			if *local {
				t.Run("local", func(t *testing.T) {
					runTest(t, &tc, "local")
				})
			}
			if *remote {
				t.Run("remote", func(t *testing.T) {
					runTest(t, &tc, "remote")
				})
			}
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
	takoBinaryDir := t.TempDir()
	t.Cleanup(func() {
		os.RemoveAll(takoBinaryDir)
	})
	takoPath := filepath.Join(takoBinaryDir, "tako")
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
		// For local mode, testCaseDir is the root, and we point to the first repo inside it
		rootPath = filepath.Join(testCaseDir, e2e.Org, tc.Repositories[0].Name)
	} else {
		// For remote mode, testCaseDir is the cloned repository root
		rootPath = testCaseDir
	}
	cacheDir := t.TempDir()
	t.Cleanup(func() {
		os.RemoveAll(cacheDir)
	})
	args := []string{"graph", "--root", rootPath, "--cache-dir", cacheDir}
	if mode == "local" {
		args = append(args, "--local")
	}
	takoCmd := exec.Command(takoPath, args...)
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
	case "simple-graph":
		return `repo-a
└── repo-b
`
	case "complex-graph":
		return `repo-a
├── repo-b
│   └── repo-c
│       └── repo-e
└── repo-d
    └── repo-e
`
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
