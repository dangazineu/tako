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

const testOrg = "tako-test"

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

	testCases := e2e.GetTestCases()
	environments := e2e.GetEnvironments(testOrg)

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.Name, func(t *testing.T) {
			for _, scenario := range scenarios {
				scenario := scenario // capture range variable
				scenarioName := fmt.Sprintf("%s/entrypoint-%s", scenario.mode, map[bool]string{true: "repo", false: "path"}[scenario.withRepoEntryPoint])
				t.Run(scenarioName, func(t *testing.T) {
					runTest(t, &tc, environments[tc.Environment], scenario.mode, scenario.withRepoEntryPoint)
				})
			}
		})
	}
}

type setupOutput struct {
	WorkDir  string `json:"workDir"`
	CacheDir string `json:"cacheDir"`
}

func runTest(t *testing.T, tc *e2e.TestCase, env e2e.TestEnvironmentDef, mode string, withRepoEntryPoint bool) {
	t.Logf("Running test case: %s", tc.Name)

	// Build binaries
	takoPath, takotestPath := buildBinaries(t)

	// Setup environment
	setupData := setupEnvironment(t, takotestPath, tc.Environment, mode, withRepoEntryPoint)
	workDir := setupData.WorkDir
	cacheDir := setupData.CacheDir

	t.Cleanup(func() {
		cleanupEnvironment(t, takotestPath, tc.Environment, mode, workDir, cacheDir)
	})

	// Run setup steps
	runSteps(t, tc.Setup, workDir, cacheDir, mode, withRepoEntryPoint, takoPath, tc, env)

	// Run test steps
	runSteps(t, tc.Test, workDir, cacheDir, mode, withRepoEntryPoint, takoPath, tc, env)

	// Run verification
	verify(t, tc, workDir, cacheDir, withRepoEntryPoint, env)
}

func verify(t *testing.T, tc *e2e.TestCase, workDir, cacheDir string, withRepoEntryPoint bool, env e2e.TestEnvironmentDef) {
	if tc.Name == "run-touch-command" {
		for _, repo := range env.Repositories {
			repoName := fmt.Sprintf("%s-%s", env.Name, repo.Name)
			var filePath string
			if withRepoEntryPoint {
				filePath = filepath.Join(cacheDir, "repos", testOrg, repoName, "test.txt")
			} else {
				if repo.Name == env.Repositories[0].Name {
					filePath = filepath.Join(workDir, repoName, "test.txt")
				} else {
					filePath = filepath.Join(cacheDir, "repos", testOrg, repoName, "test.txt")
				}
			}
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("failed to read file %s: %v", filePath, err)
			}
			if strings.TrimSpace(string(content)) != "hello" {
				t.Errorf("expected file content to be 'hello', got %q", string(content))
			}
		}
	}
}

func buildBinaries(t *testing.T) (string, string) {
	takoBinaryDir := t.TempDir()
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
	runCmd(t, exec.Command("go", "build", "-o", takoPath, "./cmd/tako"), projectRoot)
	runCmd(t, exec.Command("go", "build", "-o", takotestPath, "./cmd/takotest"), projectRoot)
	return takoPath, takotestPath
}

func setupEnvironment(t *testing.T, takotestPath, envName, mode string, withRepoEntryPoint bool) *setupOutput {
	// Create a test-specific temporary directory within the project
	testTmpDir := filepath.Join(t.TempDir(), "tako-e2e-test")
	if err := os.MkdirAll(testTmpDir, 0755); err != nil {
		t.Fatalf("failed to create test temp dir: %v", err)
	}

	workDir := filepath.Join(testTmpDir, "work")
	cacheDir := filepath.Join(testTmpDir, "cache")

	var setupArgs []string
	if mode == "local" {
		setupArgs = append(setupArgs, "--local")
	}
	if withRepoEntryPoint {
		setupArgs = append(setupArgs, "--with-repo-entrypoint")
	}

	// Add the new flags for predictable directory setup
	setupArgs = append(setupArgs, "--work-dir", workDir)
	setupArgs = append(setupArgs, "--cache-dir", cacheDir)
	setupArgs = append(setupArgs, "--owner", testOrg, envName)

	setupCmd := exec.Command(takotestPath, append([]string{"setup"}, setupArgs...)...)
	var setupOut bytes.Buffer
	setupCmd.Stdout = &setupOut
	setupCmd.Stderr = &setupOut
	if err := setupCmd.Run(); err != nil {
		t.Fatalf("failed to setup environment: %v\nOutput:\n%s", err, setupOut.String())
	}
	var setupData setupOutput
	if err := json.Unmarshal(setupOut.Bytes(), &setupData); err != nil {
		t.Fatalf("failed to unmarshal setup output: %v", err)
	}
	return &setupData
}

func cleanupEnvironment(t *testing.T, takotestPath, envName, mode, workDir, cacheDir string) {
	var cleanupArgs []string
	if mode == "local" {
		cleanupArgs = append(cleanupArgs, "--local")
	}
	cleanupArgs = append(cleanupArgs, "--owner", testOrg, envName)
	cleanupCmd := exec.Command(takotestPath, append([]string{"cleanup"}, cleanupArgs...)...)
	runCmd(t, cleanupCmd, "")
}

func runSteps(t *testing.T, steps []e2e.Step, workDir, cacheDir, mode string, withRepoEntryPoint bool, takoPath string, tc *e2e.TestCase, env e2e.TestEnvironmentDef) {
	mavenHome := os.Getenv("MAVEN_HOME")
	if mavenHome == "" {
		mavenHome = os.Getenv("M2_HOME")
	}

	originalPath := os.Getenv("PATH")
	newPath := originalPath

	if mavenHome != "" {
		newPath = fmt.Sprintf("%s/bin:%s", mavenHome, originalPath)
	} else {
		if _, err := exec.LookPath("mvn"); err != nil {
			t.Fatal("mvn command not found in PATH, and MAVEN_HOME or M2_HOME are not set")
		}
	}
	os.Setenv("PATH", newPath)
	defer os.Setenv("PATH", originalPath)

	for _, step := range steps {
		t.Run(step.Name, func(t *testing.T) {
			var cmd *exec.Cmd
			if step.Command == "tako" {
				// Set up Maven repository directory
				mavenRepoDir := filepath.Join(filepath.Dir(workDir), "maven-repo")

				args := make([]string, len(step.Args))
				copy(args, step.Args)

				// Replace Maven repository variable in tako command arguments
				for i, arg := range args {
					args[i] = strings.ReplaceAll(arg, "${MAVEN_REPO_DIR}", mavenRepoDir)
				}

				if withRepoEntryPoint {
					repoName := fmt.Sprintf("%s-%s", env.Name, env.Repositories[0].Name)
					args = append(args, "--repo", fmt.Sprintf("%s/%s:main", testOrg, repoName))
				} else {
					repoName := fmt.Sprintf("%s-%s", env.Name, env.Repositories[0].Name)
					args = append(args, "--root", filepath.Join(workDir, repoName))
				}
				if mode == "local" {
					args = append(args, "--local")
				}
				args = append(args, "--cache-dir", cacheDir)
				cmd = exec.Command(takoPath, args...)
			} else {
				// Set up Maven repository directory before processing arguments
				mavenRepoDir := filepath.Join(filepath.Dir(workDir), "maven-repo")

				args := make([]string, len(step.Args))
				for i, arg := range step.Args {
					// Special handling for template copy commands - only replace placeholders in destination path
					if step.Command == "cp" && i == 0 && strings.Contains(arg, "test/e2e/templates/") {
						args[i] = arg // Keep template source path as-is
					} else {
						args[i] = replacePathPlaceholders(arg, env, workDir, cacheDir, withRepoEntryPoint)
					}
					// Replace Maven repository variable in all arguments
					args[i] = strings.ReplaceAll(args[i], "${MAVEN_REPO_DIR}", mavenRepoDir)
				}
				cmd = exec.Command(step.Command, args...)
			}
			// Set working directory - use project root for template access, workDir for other commands
			if step.Command == "cp" && strings.Contains(strings.Join(step.Args, " "), "test/e2e/templates/") {
				// For copying from templates, use project root as working directory
				projectRoot := findProjectRoot(workDir)
				cmd.Dir = projectRoot
			} else {
				cmd.Dir = workDir
			}
			// Set Maven repo to a location within the test environment for isolation
			mavenRepoDir := filepath.Join(filepath.Dir(workDir), "maven-repo")
			cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s", newPath), fmt.Sprintf("MAVEN_REPO_DIR=%s", mavenRepoDir))

			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()

			if exitErr, ok := err.(*exec.ExitError); ok {
				if exitErr.ExitCode() != step.ExpectedExitCode {
					t.Fatalf("expected exit code %d, got %d\nOutput:\n%s", step.ExpectedExitCode, exitErr.ExitCode(), out.String())
				}
			} else if err != nil && step.ExpectedExitCode == 0 {
				t.Fatalf("command failed unexpectedly: %v\nOutput:\n%s", err, out.String())
			}

			if step.AssertOutput {
				expected := replacePlaceholders(step.ExpectedOutput, env)
				if strings.TrimSpace(out.String()) != strings.TrimSpace(expected) {
					t.Errorf("expected output to match:\n%s\ngot:\n%s", expected, out.String())
				}
			}

			if len(step.AssertOutputContains) > 0 {
				for _, s := range step.AssertOutputContains {
					if !strings.Contains(out.String(), s) {
						t.Errorf("expected output to contain %q, but it did not. Got:\n%s", s, out.String())
					}
				}
			}
		})
		if t.Failed() {
			t.Fatalf("stopping test case %s due to step failure: %s", tc.Name, step.Name)
		}
	}
}

func runCmd(t *testing.T, cmd *exec.Cmd, dir string) {
	if dir != "" {
		cmd.Dir = dir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("command failed: %v\nOutput:\n%s", err, out.String())
	}
}

func replacePlaceholders(s string, env e2e.TestEnvironmentDef) string {
	for _, repo := range env.Repositories {
		placeholder := fmt.Sprintf("{{.Repo.%s}}", repo.Name)
		fullName := fmt.Sprintf("%s-%s", env.Name, repo.Name)
		s = strings.ReplaceAll(s, placeholder, fullName)
	}
	s = strings.ReplaceAll(s, "{{.Owner}}", testOrg)
	return s
}

func replacePathPlaceholders(s string, env e2e.TestEnvironmentDef, workDir, cacheDir string, withRepoEntryPoint bool) string {
	for _, repo := range env.Repositories {
		placeholder := fmt.Sprintf("{{.Repo.%s}}", repo.Name)
		fullName := fmt.Sprintf("%s-%s", env.Name, repo.Name)

		var repoPath string
		if withRepoEntryPoint {
			repoPath = filepath.Join(cacheDir, "repos", testOrg, fullName)
		} else {
			if repo.Name == env.Repositories[0].Name {
				repoPath = filepath.Join(workDir, fullName)
			} else {
				repoPath = filepath.Join(cacheDir, "repos", testOrg, fullName)
			}
		}
		s = strings.ReplaceAll(s, placeholder, repoPath)
	}
	s = strings.ReplaceAll(s, "{{.Owner}}", testOrg)
	return s
}
