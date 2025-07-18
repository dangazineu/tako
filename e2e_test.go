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
	local                  = flag.Bool("local", false, "run local tests")
	remote                 = flag.Bool("remote", false, "run remote tests")
	withRepoEntryPoint     = flag.Bool("with-repo-entrypoint", false, "run tests with --repo flag")
	withoutRepoEntryPoint  = flag.Bool("without-repo-entrypoint", false, "run tests with --root flag")
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
	if !*withRepoEntryPoint && !*withoutRepoEntryPoint {
		t.Fatal("either -with-repo-entrypoint or -without-repo-entrypoint must be set")
	}

	for name, tc := range e2e.GetTestCases(e2e.Org) {
		name, tc := name, tc // capture range variable
		t.Run(name, func(t *testing.T) {
			if *local {
				localTC := tc
				t.Run("local", func(t *testing.T) {
					if *withoutRepoEntryPoint {
						t.Run("without-repo-entrypoint", func(t *testing.T) {
							runTest(t, &localTC, "local", "path")
						})
					}
				})
			}
			if *remote {
				remoteTC := tc
				t.Run("remote", func(t *testing.T) {
					if *withRepoEntryPoint {
						t.Run("with-repo-entrypoint", func(t *testing.T) {
							runTest(t, &remoteTC, "remote", "repo")
						})
					}
					if *withoutRepoEntryPoint {
						t.Run("without-repo-entrypoint", func(t *testing.T) {
							runTest(t, &remoteTC, "remote", "path")
						})
					}
				})
			}
		})
	}
}

func runTest(t *testing.T, tc *e2e.TestCase, mode, entrypoint string) {
	t.Logf("Running test case: %s", tc.Name)
	var testCaseDir string
	var err error

	if mode == "local" {
		testCaseDir, err = tc.SetupLocal()
		if err != nil {
			t.Fatalf("failed to setup local test case: %v", err)
		}
		t.Cleanup(func() {
			if tc.Dirty {
				os.RemoveAll(testCaseDir)
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
		tmpDir := t.TempDir()
		cmd := exec.Command("git", "clone", tc.Repositories[0].CloneURL, tmpDir)
		err = cmd.Run()
		if err != nil {
			t.Fatalf("failed to clone repo: %v", err)
		}
		testCaseDir = tmpDir
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
	cacheDir := t.TempDir()
		t.Cleanup(func() {
			os.RemoveAll(cacheDir)
	})

	var args []string
	if entrypoint == "repo" {
		args = []string{"graph", "--repo", tc.GetRepoEntryPoint(), "--cache-dir", cacheDir}
	} else {
		var rootPath string
		if mode == "local" {
			// For local mode, testCaseDir is the root, and we point to the first repo inside it
			rootPath = filepath.Join(testCaseDir, tc.Repositories[0].Owner, tc.Repositories[0].Name)
		} else {
			// For remote mode, testCaseDir is the cloned repository root
			rootPath = testCaseDir
		}
		args = []string{"graph", "--root", rootPath, "--cache-dir", cacheDir}
		if mode == "local" {
			args = append(args, "--local")
		}
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
		return "simple-graph-repo-a\n└── simple-graph-repo-b\n"
	case "complex-graph":
		return "complex-graph-repo-a\n├── complex-graph-repo-b\n│   └── complex-graph-repo-c\n│       └── complex-graph-repo-e\n└── complex-graph-repo-d\n    └── complex-graph-repo-e\n"
	case "deep-graph":
		return "deep-graph-repo-x\n└── deep-graph-repo-y\n    └── deep-graph-repo-z\n"
	case "diamond-dependency-graph":
		return "diamond-dependency-graph-repo-a\n├── diamond-dependency-graph-repo-b\n│   └── diamond-dependency-graph-repo-c\n│       └── diamond-dependency-graph-repo-e\n└── diamond-dependency-graph-repo-d\n    └── diamond-dependency-graph-repo-e\n"
	default:
		return ""
	}
}