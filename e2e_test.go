//go:build e2e
// +build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dangazineu/tako/test/e2e"
)

const testOrg = "tako-test"

var (
	local       = flag.Bool("local", false, "run local tests")
	remote      = flag.Bool("remote", false, "run remote tests")
	entrypoint  = flag.String("entrypoint", "all", "entrypoint mode to run tests in (all, path, repo)")
	preserveTmp = flag.Bool("preserve-tmp", false, "preserve temporary directories")

	// Global mutex to serialize remote test setups to avoid overwhelming GitHub API
	remoteSetupMutex sync.Mutex
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

	// Start mock GitHub server if test case requires it
	var mockServer *e2e.MockGitHubServer
	if requiresMockServer(tc) {
		mockServer = startMockGitHubServer(t)
	}

	t.Cleanup(func() {
		if mockServer != nil {
			mockServer.Stop()
		}
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
	// Use the data-driven verification from the test case
	for _, fileCheck := range tc.Verify.Files {
		// Only check the primary repository (first repository in the environment)
		// This is where the workflow is triggered and files are created
		repo := env.Repositories[0]
		repoName := fmt.Sprintf("%s-%s", env.Name, repo.Name)
		var filePath string
		if withRepoEntryPoint {
			filePath = filepath.Join(cacheDir, "repos", testOrg, repoName, repo.Branch, fileCheck.FileName)
		} else {
			filePath = filepath.Join(workDir, repoName, fileCheck.FileName)
		}

		if fileCheck.ShouldExist {
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Errorf("expected file %s to exist but got error: %v", filePath, err)
				continue
			}
			if fileCheck.ExpectedContent != "" {
				actualContent := strings.TrimSpace(string(content))
				if actualContent != fileCheck.ExpectedContent {
					t.Errorf("file %s: expected content %q, got %q", filePath, fileCheck.ExpectedContent, actualContent)
				}
			}
		} else {
			// Verify the file does NOT exist
			if _, err := os.Stat(filePath); err == nil {
				t.Errorf("file %s should not exist", filePath)
			} else if !os.IsNotExist(err) {
				t.Errorf("unexpected error checking file %s: %v", filePath, err)
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

	// Use longer timeout for setup in remote mode (especially for Maven builds and GitHub API delays)
	timeout := 5 * time.Minute
	if mode == "remote" {
		timeout = 20 * time.Minute // Increased timeout for GitHub API rate limits
		// Serialize remote setups to avoid overwhelming GitHub API
		remoteSetupMutex.Lock()
		defer remoteSetupMutex.Unlock()
		// Add delay between remote test setups to respect GitHub rate limits
		time.Sleep(5 * time.Second)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	setupCmd = exec.CommandContext(ctx, setupCmd.Path, setupCmd.Args[1:]...)
	setupCmd.Stdout = &setupOut
	setupCmd.Stderr = &setupOut

	// Add retry logic for remote setup failures
	maxRetries := 2 // Reduce retries since we'll use longer wait times
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = setupCmd.Run()
		if err == nil {
			break
		}

		// Check if it's a rate limit error
		if mode == "remote" && strings.Contains(setupOut.String(), "rate limit") {
			if attempt < maxRetries {
				// For secondary rate limits, GitHub typically blocks for 1-5 minutes
				// Use progressively longer wait times
				waitTime := time.Duration(attempt*2) * time.Minute
				t.Logf("Setup failed due to GitHub secondary rate limit (attempt %d/%d), waiting %v for rate limit to reset", attempt, maxRetries, waitTime)
				time.Sleep(waitTime)

				// Reset the command and output buffer for retry
				setupOut.Reset()
				setupCmd = exec.CommandContext(ctx, takotestPath, append([]string{"setup"}, setupArgs...)...)
				setupCmd.Stdout = &setupOut
				setupCmd.Stderr = &setupOut
				continue
			} else {
				// On final attempt, provide helpful information
				t.Logf("Remote tests are currently blocked by GitHub's secondary rate limit. This is expected when running multiple remote tests in succession.")
				t.Logf("Consider running tests with longer intervals or focus on local tests which don't have this limitation.")
			}
		}
		break
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("setup timed out after %v: %s\nOutput:\n%s", timeout, strings.Join(setupCmd.Args, " "), setupOut.String())
		}
		t.Fatalf("failed to setup environment after %d attempts: %v\nOutput:\n%s", maxRetries, err, setupOut.String())
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
	if *preserveTmp {
		cleanupArgs = append(cleanupArgs, "--preserve-tmp")
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

			// Determine timeout based on command type
			timeout := 5 * time.Minute
			if step.Command == "mvn" || (step.Command == "tako" && len(step.Args) > 1 && strings.Contains(step.Args[1], "mvn")) {
				timeout = 15 * time.Minute // Longer timeout for Maven builds
			}

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			// Create new command with context
			cmdWithTimeout := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
			cmdWithTimeout.Dir = cmd.Dir
			cmdWithTimeout.Env = cmd.Env

			var out bytes.Buffer
			cmdWithTimeout.Stdout = &out
			cmdWithTimeout.Stderr = &out
			err := cmdWithTimeout.Run()

			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("command timed out after %v: %s\nOutput:\n%s", timeout, strings.Join(cmd.Args, " "), out.String())
			}

			if exitErr, ok := err.(*exec.ExitError); ok {
				if exitErr.ExitCode() != step.ExpectedExitCode {
					t.Fatalf("expected exit code %d, got %d\nOutput:\n%s", step.ExpectedExitCode, exitErr.ExitCode(), out.String())
				}
			} else if err != nil && step.ExpectedExitCode == 0 {
				t.Fatalf("command failed unexpectedly: %v\nOutput:\n%s", err, out.String())
			}

			if step.AssertOutput {
				expected := replacePlaceholders(step.ExpectedOutput, env, withRepoEntryPoint)
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
	runCmdWithTimeout(t, cmd, dir, 5*time.Minute) // Default 5 minute timeout
}

func runCmdWithTimeout(t *testing.T, cmd *exec.Cmd, dir string, timeout time.Duration) {
	if dir != "" {
		cmd.Dir = dir
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Set context on command
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("command timed out after %v: %s\nOutput:\n%s", timeout, strings.Join(cmd.Args, " "), out.String())
		}
		t.Fatalf("command failed: %v\nOutput:\n%s", err, out.String())
	}
}

func replacePlaceholders(s string, env e2e.TestEnvironmentDef, withRepoEntryPoint bool) string {
	for _, repo := range env.Repositories {
		placeholder := fmt.Sprintf("{{.Repo.%s}}", repo.Name)
		fullName := fmt.Sprintf("%s/%s-%s", testOrg, env.Name, repo.Name)
		s = strings.ReplaceAll(s, placeholder, fullName)
	}
	s = strings.ReplaceAll(s, "{{.Owner}}", testOrg)

	// Handle conditional repository line for exec command
	if withRepoEntryPoint {
		// Replace placeholder with actual repository line
		repoName := fmt.Sprintf("%s/%s-%s:main", testOrg, env.Name, env.Repositories[0].Name)
		s = strings.ReplaceAll(s, "{{.RepoLine}}", fmt.Sprintf("Repository: %s\n", repoName))
	} else {
		// Remove repository line placeholder when not using repo entrypoint
		s = strings.ReplaceAll(s, "{{.RepoLine}}", "")
	}

	return s
}

func replacePathPlaceholders(s string, env e2e.TestEnvironmentDef, workDir, cacheDir string, withRepoEntryPoint bool) string {
	for _, repo := range env.Repositories {
		placeholder := fmt.Sprintf("{{.Repo.%s}}", repo.Name)
		fullName := fmt.Sprintf("%s-%s", env.Name, repo.Name)

		var repoPath string
		if withRepoEntryPoint {
			repoPath = filepath.Join(cacheDir, "repos", testOrg, fullName, repo.Branch)
		} else {
			if repo.Name == env.Repositories[0].Name {
				repoPath = filepath.Join(workDir, fullName)
			} else {
				repoPath = filepath.Join(cacheDir, "repos", testOrg, fullName, repo.Branch)
			}
		}
		s = strings.ReplaceAll(s, placeholder, repoPath)
	}
	s = strings.ReplaceAll(s, "{{.Owner}}", testOrg)
	return s
}

// requiresMockServer checks if a test case requires the mock GitHub server
func requiresMockServer(tc *e2e.TestCase) bool {
	// Check if any setup step mentions starting a mock server
	for _, step := range tc.Setup {
		if strings.Contains(step.Name, "mock github server") {
			return true
		}
	}

	// Check if test case name suggests it needs mock server
	return strings.Contains(tc.Name, "java-bom-fanout")
}

// startMockGitHubServer starts the mock GitHub server and sets up environment variables
func startMockGitHubServer(t *testing.T) *e2e.MockGitHubServer {
	mockServer := e2e.NewMockGitHubServer()

	// Start server in background
	go func() {
		if err := mockServer.Start(8080); err != nil && err != http.ErrServerClosed {
			t.Logf("Mock GitHub server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Health check
	resp, err := http.Get("http://localhost:8080/health")
	if err != nil {
		t.Fatalf("Mock GitHub server health check failed: %v", err)
	}
	resp.Body.Close()

	// Set environment variables for mock CLI tools
	originalAPIURL := os.Getenv("GITHUB_API_URL")
	originalOwner := os.Getenv("REPO_OWNER")

	os.Setenv("GITHUB_API_URL", "http://localhost:8080")
	os.Setenv("REPO_OWNER", testOrg)

	// Restore environment on cleanup
	t.Cleanup(func() {
		if originalAPIURL != "" {
			os.Setenv("GITHUB_API_URL", originalAPIURL)
		} else {
			os.Unsetenv("GITHUB_API_URL")
		}
		if originalOwner != "" {
			os.Setenv("REPO_OWNER", originalOwner)
		} else {
			os.Unsetenv("REPO_OWNER")
		}
	})

	t.Logf("Mock GitHub server started on http://localhost:8080")

	// Start CI simulation in background
	go simulateCI(t, mockServer)

	return mockServer
}

// simulateCI simulates CI completion for PRs created during the test
func simulateCI(t *testing.T, mockServer *e2e.MockGitHubServer) {
	// Poll for new PRs and complete their CI checks
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	processedPRs := make(map[string]bool)

	for {
		select {
		case <-ticker.C:
			// Find all PRs across all repos and mark CI as complete
			for _, repo := range []string{"java-bom-fanout-core-lib", "java-bom-fanout-lib-a", "java-bom-fanout-lib-b", "java-bom-fanout-java-bom"} {
				prs := mockServer.ListPRs(testOrg, repo)
				for _, pr := range prs {
					prKey := fmt.Sprintf("%s/%s/%d", testOrg, repo, pr.Number)
					if pr.State == "open" && pr.CheckStatus == "pending" && !processedPRs[prKey] {
						// Simulate CI taking some time (1-3 seconds)
						time.Sleep(time.Duration(1+pr.Number%3) * time.Second)

						// Mark CI as complete
						completeURL := fmt.Sprintf("http://localhost:8080/test/ci/%s/%s/%d/complete", testOrg, repo, pr.Number)
						resp, err := http.Post(completeURL, "application/json", nil)
						if err == nil {
							resp.Body.Close()
							processedPRs[prKey] = true
							t.Logf("Simulated CI completion for PR #%d in %s/%s", pr.Number, testOrg, repo)
						} else {
							t.Logf("Failed to complete CI for PR #%d in %s/%s: %v", pr.Number, testOrg, repo, err)
						}
					}
				}
			}
		case <-time.After(5 * time.Minute):
			// Stop after 5 minutes (test timeout)
			return
		}
	}
}
