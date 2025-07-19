//go:build e2e
// +build e2e

package main

import (
	"bytes"
	"flag"
	"fmt"
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
	for name, tc := range e2e.GetTestCases(e2e.Org) {
		name, tc := name, tc // capture range variable
		t.Run(name, func(t *testing.T) {
			if *local {
				localTC := tc
				t.Run("local", func(t *testing.T) {
					runTest(t, &localTC, "local")
				})
			}
			if *remote {
				remoteTC := tc
				t.Run("remote", func(t *testing.T) {
					runTest(t, &remoteTC, "remote")
				})
			}
		})
	}
}

func runTest(t *testing.T, tc *e2e.TestCase, mode string) {
	t.Logf("Running test case: %s", tc.Name)
	var cacheDir, workDir string
	var err error

	if mode == "local" {
		testCaseBaseDir, err := tc.SetupLocal()
		if err != nil {
			t.Fatalf("failed to setup local test case: %v", err)
		}
		cacheDir = filepath.Join(testCaseBaseDir, "cache")
		workDir = filepath.Join(testCaseBaseDir, "workdir")
		t.Cleanup(func() {
			if tc.Dirty {
				os.RemoveAll(testCaseBaseDir)
			}
		})
	} else {
		client, err := e2e.GetClient()
		if err != nil {
			t.Fatalf("failed to get github client: %v", err)
		}
		if err := tc.Setup(client); err != nil {
			t.Fatalf("failed to setup remote test case: %v", err)
		}

		cacheDir = t.TempDir()
		workDir = t.TempDir()

		if !tc.WithRepoEntryPoint {
			cmd := exec.Command("git", "clone", tc.Repositories[0].CloneURL, workDir)
			err = cmd.Run()
			if err != nil {
				t.Fatalf("failed to clone repo: %v", err)
			}
		}
		t.Cleanup(func() {
			if err := tc.Cleanup(client); err != nil {
				t.Errorf("failed to cleanup remote test case: %v", err)
			}
		})
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
	var args []string

	if tc.WithRepoEntryPoint {
		args = []string{"graph", "--repo", fmt.Sprintf("%s/%s:main", tc.Repositories[0].Owner, tc.Repositories[0].Name), "--cache-dir", cacheDir}
	} else {
		var rootPath string
		if mode == "local" {
			rootPath = filepath.Join(workDir, tc.Repositories[0].Name)
		} else {
			rootPath = workDir
		}
		args = []string{"graph", "--root", rootPath, "--cache-dir", cacheDir}
	}

	if mode == "local" {
		args = append(args, "--local")
	}
	takoCmd := exec.Command(takoPath, args...)
	if !tc.WithRepoEntryPoint {
		if mode == "local" {
			takoCmd.Dir = filepath.Join(workDir, tc.Repositories[0].Name)
		} else {
			takoCmd.Dir = workDir
		}
	} else {
		takoCmd.Dir = workDir
	}

	files, err := os.ReadDir(workDir)
	if err != nil {
		t.Logf("could not read workdir: %v", err)
	}
	for _, file := range files {
		t.Logf("workdir content: %s", file.Name())
	}

	t.Logf("takoCmd.Dir: %s", takoCmd.Dir)
	if _, err := os.Stat(takoCmd.Dir); os.IsNotExist(err) {
		t.Fatalf("takoCmd.Dir does not exist: %s", takoCmd.Dir)
	}

	takoCmd.Stdout = &out
	takoCmd.Stderr = &out
	err = takoCmd.Run()

	if tc.ExpectedError != "" {
		if err == nil {
			t.Fatalf("expected to fail with error %q, but it succeeded", tc.ExpectedError)
		}
		if !strings.Contains(out.String(), tc.ExpectedError) {
			t.Errorf("expected output to contain %q, got %q", tc.ExpectedError, out.String())
		}
		return
	}

	if err != nil {
		t.Fatalf("failed to run tako graph: %v\nOutput:\n%s", err, out.String())
	}

	// Assert the output
	expected := getExpectedOutput(tc.Name)
	if testing.Verbose() {
		t.Logf("Expected output:\n%s", expected)
		t.Logf("Actual output:\n%s", out.String())
		t.Logf("trimmed expected: %q", strings.TrimSpace(expected))
		t.Logf("trimmed actual: %q", strings.TrimSpace(out.String()))
	}
	if strings.TrimSpace(out.String()) != strings.TrimSpace(expected) {
		t.Errorf("expected output to not match %q, got %q", expected, out.String())
	}
}

func getExpectedOutput(testCaseName string) string {
	switch testCaseName {
	case "simple-graph", "simple-graph-with-repo-flag":
		return `repo-a
└── repo-b`
	case "complex-graph":
		return `repo-a
├── repo-b
│   └── repo-c
│       └── repo-e
└── repo-d
    └── repo-e`
	case "deep-graph":
		return `repo-x
└── repo-y
    └── repo-z`
	case "diamond-dependency-graph":
		return `repo-a
├── repo-b
│   └── repo-c
│       └── repo-e
└── repo-d
    └── repo-e`
	default:
		return ""
	}
}
