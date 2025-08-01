//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestJavaBOMFanout runs the comprehensive Java BOM E2E test with mock GitHub server
func TestJavaBOMFanout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Java BOM fanout E2E test in short mode")
	}

	// Start mock GitHub server
	mockServer := NewMockGitHubServer()
	go func() {
		if err := mockServer.Start(8080); err != nil && err != http.ErrServerClosed {
			t.Errorf("Mock GitHub server failed: %v", err)
		}
	}()
	defer mockServer.Stop()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Health check
	resp, err := http.Get("http://localhost:8080/health")
	if err != nil {
		t.Fatalf("Mock GitHub server health check failed: %v", err)
	}
	resp.Body.Close()

	// Set environment variables for test
	originalAPIURL := os.Getenv("GITHUB_API_URL")
	originalOwner := os.Getenv("REPO_OWNER")

	os.Setenv("GITHUB_API_URL", "http://localhost:8080")
	os.Setenv("REPO_OWNER", "testorg")

	defer func() {
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
	}()

	// Run the base E2E test framework with our test case
	testCases := []TestCase{
		getJavaBOMFanoutTestCase(),
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()

			// Create test orchestrator
			orchestrator := &JavaBOMTestOrchestrator{
				t:          t,
				mockServer: mockServer,
			}

			// Run the enhanced test with orchestration
			err := orchestrator.RunTest(ctx, testCase)
			if err != nil {
				t.Fatalf("Java BOM fanout test failed: %v", err)
			}
		})
	}
}

// JavaBOMTestOrchestrator handles the complex orchestration for the Java BOM test
type JavaBOMTestOrchestrator struct {
	t          *testing.T
	mockServer *MockGitHubServer
}

// RunTest executes the Java BOM test with proper CI simulation
func (o *JavaBOMTestOrchestrator) RunTest(ctx context.Context, testCase TestCase) error {
	// Set up the standard E2E test environment
	tempDir, cleanup, err := setupTestEnvironment(testCase.Environment, testCase.ReadOnly)
	if err != nil {
		return fmt.Errorf("failed to setup test environment: %w", err)
	}
	defer cleanup()

	// Change to the core-lib directory to start the test
	coreLibDir := filepath.Join(tempDir, "java-bom-fanout-core-lib")
	originalDir, _ := os.Getwd()
	os.Chdir(coreLibDir)
	defer os.Chdir(originalDir)

	// Create a monitoring goroutine for PR orchestration
	go o.monitorAndOrchestratePRs(ctx)

	// Execute the test steps
	for _, step := range testCase.Test {
		if err := o.executeStep(step, coreLibDir); err != nil {
			return fmt.Errorf("step '%s' failed: %w", step.Name, err)
		}

		// Add delay for async workflows to complete
		time.Sleep(2 * time.Second)
	}

	// Wait for all workflows to complete
	if err := o.waitForWorkflowCompletion(10 * time.Minute); err != nil {
		return fmt.Errorf("workflows did not complete in time: %w", err)
	}

	// Verify results
	return o.verifyResults(testCase.Verify, tempDir)
}

// monitorAndOrchestratePRs handles CI simulation and PR orchestration
func (o *JavaBOMTestOrchestrator) monitorAndOrchestratePRs(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	processedPRs := make(map[string]bool)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check for new PRs across all repositories
			repos := []string{"java-bom-fanout-core-lib", "java-bom-fanout-lib-a", "java-bom-fanout-lib-b", "java-bom-fanout-java-bom"}

			for _, repo := range repos {
				prs := o.mockServer.ListPRs("testorg", repo)

				for _, pr := range prs {
					prKey := fmt.Sprintf("%s/%s/%d", pr.Owner, pr.Repo, pr.Number)

					if processedPRs[prKey] || pr.State != "open" {
						continue
					}

					// Simulate CI taking some time
					time.Sleep(1 * time.Second)

					// Mark CI as complete for this PR
					err := o.completeCIForPR(pr.Owner, pr.Repo, pr.Number)
					if err != nil {
						o.t.Logf("Failed to complete CI for PR %d in %s/%s: %v", pr.Number, pr.Owner, pr.Repo, err)
						continue
					}

					processedPRs[prKey] = true
					o.t.Logf("Completed CI simulation for PR #%d in %s/%s", pr.Number, pr.Owner, pr.Repo)
				}
			}
		}
	}
}

// completeCIForPR marks CI as complete for a specific PR
func (o *JavaBOMTestOrchestrator) completeCIForPR(owner, repo string, prNumber int) error {
	url := fmt.Sprintf("http://localhost:8080/test/ci/%s/%s/%d/complete", owner, repo, prNumber)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to complete CI: status %d", resp.StatusCode)
	}

	return nil
}

// executeStep executes a single test step
func (o *JavaBOMTestOrchestrator) executeStep(step Step, workDir string) error {
	// This would integrate with the existing E2E test execution logic
	// For now, we'll simulate the workflow execution
	o.t.Logf("Executing step: %s", step.Name)

	if step.Command == "tako" && len(step.Args) > 0 && step.Args[0] == "exec" {
		// This is a workflow execution - simulate it
		o.t.Logf("Simulating workflow execution: %s", strings.Join(step.Args, " "))

		// In a real implementation, this would call the actual tako command
		// and integrate with the existing E2E test framework
		return nil
	}

	return nil
}

// waitForWorkflowCompletion waits for all workflows to complete their execution
func (o *JavaBOMTestOrchestrator) waitForWorkflowCompletion(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for workflow completion")
		case <-ticker.C:
			// Check if all expected PRs have been processed
			// This is a simplified check - in practice, we'd monitor workflow states
			if o.areAllWorkflowsComplete() {
				return nil
			}
		}
	}
}

// areAllWorkflowsComplete checks if all expected workflows have completed
func (o *JavaBOMTestOrchestrator) areAllWorkflowsComplete() bool {
	// Check that we have the expected number of PRs created and merged
	expectedRepos := []string{"java-bom-fanout-lib-a", "java-bom-fanout-lib-b", "java-bom-fanout-java-bom"}

	for _, repo := range expectedRepos {
		prs := o.mockServer.ListPRs("testorg", repo)
		hasMergedPR := false

		for _, pr := range prs {
			if pr.State == "merged" {
				hasMergedPR = true
				break
			}
		}

		if !hasMergedPR {
			return false
		}
	}

	return true
}

// verifyResults verifies the test results against expected outcomes
func (o *JavaBOMTestOrchestrator) verifyResults(verification Verification, tempDir string) error {
	// Verify files exist in the appropriate repositories
	for _, file := range verification.Files {
		// Determine which repository should contain this file
		repoDir := o.determineRepositoryForFile(file.FileName, tempDir)
		filePath := filepath.Join(repoDir, file.FileName)

		if file.ShouldExist {
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				return fmt.Errorf("expected file %s does not exist in %s", file.FileName, repoDir)
			}

			if file.ExpectedContent != "" {
				content, err := os.ReadFile(filePath)
				if err != nil {
					return fmt.Errorf("failed to read file %s: %w", filePath, err)
				}

				if strings.TrimSpace(string(content)) != file.ExpectedContent {
					return fmt.Errorf("file %s content mismatch: expected %s, got %s",
						filePath, file.ExpectedContent, strings.TrimSpace(string(content)))
				}
			}
		} else {
			if _, err := os.Stat(filePath); !os.IsNotExist(err) {
				return fmt.Errorf("file %s should not exist but does in %s", file.FileName, repoDir)
			}
		}
	}

	// Verify PR orchestration worked correctly
	return o.verifyPROrchestration()
}

// determineRepositoryForFile determines which repository should contain a specific file
func (o *JavaBOMTestOrchestrator) determineRepositoryForFile(fileName, tempDir string) string {
	if strings.Contains(fileName, "core-lib") {
		return filepath.Join(tempDir, "java-bom-fanout-core-lib")
	}
	if strings.Contains(fileName, "lib-a") {
		return filepath.Join(tempDir, "java-bom-fanout-lib-a")
	}
	if strings.Contains(fileName, "lib-b") {
		return filepath.Join(tempDir, "java-bom-fanout-lib-b")
	}
	if strings.Contains(fileName, "java-bom") || strings.Contains(fileName, "bom") {
		return filepath.Join(tempDir, "java-bom-fanout-java-bom")
	}

	// Default to core-lib if we can't determine
	return filepath.Join(tempDir, "java-bom-fanout-core-lib")
}

// verifyPROrchestration verifies that the PR orchestration worked as expected
func (o *JavaBOMTestOrchestrator) verifyPROrchestration() error {
	// Verify lib-a PRs
	libAPRs := o.mockServer.ListPRs("testorg", "java-bom-fanout-lib-a")
	if len(libAPRs) == 0 {
		return fmt.Errorf("no PRs found for lib-a")
	}

	// Verify lib-b PRs
	libBPRs := o.mockServer.ListPRs("testorg", "java-bom-fanout-lib-b")
	if len(libBPRs) == 0 {
		return fmt.Errorf("no PRs found for lib-b")
	}

	// Verify BOM PRs
	bomPRs := o.mockServer.ListPRs("testorg", "java-bom-fanout-java-bom")
	if len(bomPRs) == 0 {
		return fmt.Errorf("no PRs found for java-bom")
	}

	// Verify all PRs were merged
	allPRs := append(append(libAPRs, libBPRs...), bomPRs...)
	for _, pr := range allPRs {
		if pr.State != "merged" {
			return fmt.Errorf("PR #%d in %s/%s was not merged (state: %s)",
				pr.Number, pr.Owner, pr.Repo, pr.State)
		}
	}

	o.t.Logf("Successfully verified PR orchestration: %d PRs created and merged", len(allPRs))
	return nil
}

// getJavaBOMFanoutTestCase returns the test case for Java BOM fanout
func getJavaBOMFanoutTestCase() TestCase {
	// Get the test case from the standard test cases
	testCases := GetTestCases()
	for _, tc := range testCases {
		if tc.Name == "java-bom-fanout" {
			return tc
		}
	}

	// Fallback if not found
	return TestCase{
		Name:        "java-bom-fanout",
		Environment: "java-bom-fanout",
		ReadOnly:    false,
	}
}

// setupTestEnvironment sets up the test environment (placeholder for integration with existing E2E framework)
func setupTestEnvironment(environment string, readOnly bool) (string, func(), error) {
	// This would integrate with the existing E2E test environment setup
	// For now, return a placeholder
	tempDir := "/tmp/java-bom-test"
	cleanup := func() {
		// Cleanup logic
	}
	return tempDir, cleanup, nil
}
