//go:build e2e
// +build e2e

package main

import (
	"bytes"
	"encoding/json"
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
	local      = flag.Bool("local", false, "run local tests")
	remote     = flag.Bool("remote", false, "run remote tests")
	entrypoint = flag.String("entrypoint", "all", "entrypoint mode to run tests in (all, path, repo)")
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

	scenarios := []struct {
		mode               string
		withRepoEntryPoint bool
	}{}

	if *local {
		if *entrypoint == "all" || *entrypoint == "path" {
			scenarios = append(scenarios, struct {
				mode               string
				withRepoEntryPoint bool
			}{"local", false})
		}
		if *entrypoint == "all" || *entrypoint == "repo" {
			scenarios = append(scenarios, struct {
				mode               string
				withRepoEntryPoint bool
			}{"local", true})
		}
	}
	if *remote {
		if *entrypoint == "all" || *entrypoint == "path" {
			scenarios = append(scenarios, struct {
				mode               string
				withRepoEntryPoint bool
			}{"remote", false})
		}
		if *entrypoint == "all" || *entrypoint == "repo" {
			scenarios = append(scenarios, struct {
				mode               string
				withRepoEntryPoint bool
			}{"remote", true})
		}
	}

	for name, tc := range e2e.GetTestCases(e2e.Org) {
		name, tc := name, tc // capture range variable
		t.Run(name, func(t *testing.T) {
			for _, scenario := range scenarios {
				scenario := scenario // capture range variable
				scenarioName := fmt.Sprintf("%s/entrypoint-%s", scenario.mode, map[bool]string{true: "repo", false: "path"}[scenario.withRepoEntryPoint])
				t.Run(scenarioName, func(t *testing.T) {
					runTest(t, &tc, scenario.mode, scenario.withRepoEntryPoint)
				})
			}
		})
	}
}

type setupOutput struct {
	WorkDir  string `json:"workDir"`
	CacheDir string `json:"cacheDir"`
}

func runTest(t *testing.T, tc *e2e.TestCase, mode string, withRepoEntryPoint bool) {
	t.Logf("Running test case: %s", tc.Name)

	runCmd := func(t *testing.T, cmd *exec.Cmd) {
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		if err != nil {
			if testing.Verbose() {
				t.Logf("command failed: %v\nOutput:\n%s", err, out.String())
			}
			t.Fatalf("command failed: %v", err)
		}
	}

	// Build the tako and takotest binaries
	takoBinaryDir := t.TempDir()
	t.Cleanup(func() {
		os.RemoveAll(takoBinaryDir)
	})
	takoPath := filepath.Join(takoBinaryDir, "tako")
	takotestPath := filepath.Join(takoBinaryDir, "takotest")
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
	runCmd(t, buildCmd)
	buildCmd = exec.Command("go", "build", "-o", takotestPath, "./cmd/takotest")
	buildCmd.Dir = projectRoot
	runCmd(t, buildCmd)

	// Setup the test case
	var setupArgs []string
	if mode == "local" {
		setupArgs = append(setupArgs, "--local")
	}
	if withRepoEntryPoint {
		setupArgs = append(setupArgs, "--with-repo-entrypoint")
	}
	setupArgs = append(setupArgs, "--owner", e2e.Org, tc.Name)
	setupCmd := exec.Command(takotestPath, append([]string{"setup"}, setupArgs...)...)
	var setupOut bytes.Buffer
	setupCmd.Stdout = &setupOut
	setupCmd.Stderr = &setupOut
	err = setupCmd.Run()
	if err != nil {
		if testing.Verbose() {
			t.Logf("failed to setup test case: %v\nOutput:\n%s", err, setupOut.String())
		}
		t.Fatalf("failed to setup test case: %v", err)
	}
	var setupData setupOutput
	err = json.Unmarshal(setupOut.Bytes(), &setupData)
	if err != nil {
		t.Fatalf("failed to unmarshal setup output: %v", err)
	}
	workDir := setupData.WorkDir
	cacheDir := setupData.CacheDir

	t.Cleanup(func() {
		var cleanupArgs []string
		if mode == "local" {
			cleanupArgs = append(cleanupArgs, "--local")
		}
		cleanupArgs = append(cleanupArgs, "--owner", e2e.Org, "--work-dir", workDir, "--cache-dir", cacheDir, tc.Name)
		cleanupCmd := exec.Command(takotestPath, append([]string{"cleanup"}, cleanupArgs...)...)
		runCmd(t, cleanupCmd)
	})

	// Run tako graph
	var args []string

	if withRepoEntryPoint {
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
	if !withRepoEntryPoint {
		if mode == "local" {
			takoCmd.Dir = filepath.Join(workDir, tc.Repositories[0].Name)
		} else {
			takoCmd.Dir = workDir
		}
	} else {
		takoCmd.Dir = workDir
	}

	var out bytes.Buffer
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
		if testing.Verbose() {
			t.Logf("failed to run tako graph: %v\nOutput:\n%s", err, out.String())
		}
		t.Fatalf("failed to run tako graph: %v", err)
	}

	// Assert the output
	expected := getExpectedOutput(tc)
	if strings.TrimSpace(out.String()) != strings.TrimSpace(expected) {
		if testing.Verbose() {
			t.Logf("Expected output:\n%s", expected)
			t.Logf("Actual output:\n%s", out.String())
		}
		t.Errorf("expected output to not match %q, got %q", expected, out.String())
	}
}

func getExpectedOutput(tc *e2e.TestCase) string {
	var expected string
	switch tc.Name {
	case "simple-graph":
		expected = `{{repo-a}}
└── {{repo-b}}
`
	case "complex-graph":
		expected = `{{repo-a}}
├── {{repo-b}}
│   └── {{repo-c}}
│       └── {{repo-e}}
└── {{repo-d}}
    └── {{repo-e}}
`
	case "deep-graph":
		expected = `{{repo-x}}
└── {{repo-y}}
    └── {{repo-z}}
`
	case "diamond-dependency-graph":
		expected = `{{repo-a}}
├── {{repo-b}}
│   └── {{repo-c}}
│       └── {{repo-e}}
└── {{repo-d}}
    └── {{repo-e}}
`
	default:
		return ""
	}

	for _, repo := range tc.Repositories {
		originalName := strings.ReplaceAll(repo.Name, tc.Name+"-", "")
		expected = strings.ReplaceAll(expected, fmt.Sprintf("{{%s}}", originalName), repo.Name)
	}
	return expected
}