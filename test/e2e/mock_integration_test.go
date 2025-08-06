//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestMockGitHubIntegration tests the integration between mock GitHub server and CLI
func TestMockGitHubIntegration(t *testing.T) {
	t.Skip("Skipping test that depends on java-bom-fanout mock tools which are currently disabled")
	if testing.Short() {
		t.Skip("Skipping mock integration test in short mode")
	}

	// Start mock GitHub server
	mockServer := NewMockGitHubServer()
	go func() {
		if err := mockServer.Start(8081); err != nil && err != http.ErrServerClosed {
			t.Errorf("Mock GitHub server failed: %v", err)
		}
	}()
	defer mockServer.Stop()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Health check
	resp, err := http.Get("http://localhost:8081/health")
	if err != nil {
		t.Fatalf("Mock GitHub server health check failed: %v", err)
	}
	resp.Body.Close()

	// Set environment variables for mock CLI
	originalAPIURL := os.Getenv("GITHUB_API_URL")
	originalOwner := os.Getenv("REPO_OWNER")

	os.Setenv("GITHUB_API_URL", "http://localhost:8081")
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

	// Create a temporary directory to simulate a repository
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "test-repo")
	err = os.MkdirAll(repoDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test repo directory: %v", err)
	}

	// Change to the repository directory
	originalDir, _ := os.Getwd()
	os.Chdir(repoDir)
	defer os.Chdir(originalDir)

	// Initialize git repository
	exec.Command("git", "init").Run()
	exec.Command("git", "config", "user.name", "Test User").Run()
	exec.Command("git", "config", "user.email", "test@example.com").Run()

	// Create initial commit
	err = os.WriteFile("README.md", []byte("# Test Repository"), 0644)
	if err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}
	exec.Command("git", "add", "README.md").Run()
	exec.Command("git", "commit", "-m", "Initial commit").Run()

	// Test PR creation workflow
	t.Run("PR Creation and Management", func(t *testing.T) {
		// Test PR creation
		mockGHPath := "/Users/danielgazineu/dev/workspace/tako/test/e2e/templates/java-bom-fanout/mock-gh.sh"
		cmd := exec.Command(mockGHPath, "pr", "create", "--title", "Test PR", "--body", "Test PR body")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err != nil {
			t.Fatalf("PR creation failed: %v, stderr: %s", err, stderr.String())
		}

		output := stdout.String()
		t.Logf("PR creation output: %s", output)

		// Extract PR number from output (should be the last line)
		lines := bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n"))
		if len(lines) == 0 {
			t.Fatal("No output from PR creation")
		}
		prNumber := string(lines[len(lines)-1])
		t.Logf("Created PR #%s", prNumber)

		// Verify PR was created in mock server
		prs := mockServer.ListPRs("testorg", "test-repo")
		if len(prs) != 1 {
			t.Fatalf("Expected 1 PR, got %d", len(prs))
		}

		pr := prs[0]
		if pr.Title != "Test PR" {
			t.Errorf("Expected PR title 'Test PR', got '%s'", pr.Title)
		}
		if pr.State != "open" {
			t.Errorf("Expected PR state 'open', got '%s'", pr.State)
		}
		if pr.CheckStatus != "pending" {
			t.Errorf("Expected check status 'pending', got '%s'", pr.CheckStatus)
		}

		// Test CI check completion (simulate by calling server endpoint)
		completeURL := "http://localhost:8081/test/ci/testorg/test-repo/" + prNumber + "/complete"
		resp, err := http.Post(completeURL, "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to complete CI: %v", err)
		}
		resp.Body.Close()

		// Verify CI status changed
		pr = mockServer.GetPR("testorg", "test-repo", pr.Number)
		if pr.CheckStatus != "success" {
			t.Errorf("Expected check status 'success' after completion, got '%s'", pr.CheckStatus)
		}

		// Test PR merge
		cmd = exec.Command(mockGHPath, "pr", "merge", prNumber, "--squash", "--delete-branch")
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		stdout.Reset()
		stderr.Reset()

		err = cmd.Run()
		if err != nil {
			t.Fatalf("PR merge failed: %v, stderr: %s", err, stderr.String())
		}

		// Verify PR was merged
		pr = mockServer.GetPR("testorg", "test-repo", pr.Number)
		if pr.State != "merged" {
			t.Errorf("Expected PR state 'merged', got '%s'", pr.State)
		}

		t.Logf("Successfully completed PR lifecycle: create -> CI -> merge")
	})

	// Test blocking watch behavior
	t.Run("PR Checks Watch Blocking", func(t *testing.T) {
		// Reset server state
		http.Post("http://localhost:8081/test/reset", "application/json", nil)

		// Create another PR
		mockGHPath := "/Users/danielgazineu/dev/workspace/tako/test/e2e/templates/java-bom-fanout/mock-gh.sh"
		cmd := exec.Command(mockGHPath, "pr", "create", "--title", "Watch Test PR", "--body", "Testing watch behavior")
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("PR creation failed: %v", err)
		}

		// Extract PR number
		lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
		prNumber := string(lines[len(lines)-1])

		// Start watch command in background
		watchCmd := exec.Command(mockGHPath, "pr", "checks", prNumber, "--watch")
		watchCmd.Dir = repoDir

		// Use a channel to track when watch completes
		watchDone := make(chan error, 1)
		go func() {
			watchDone <- watchCmd.Run()
		}()

		// Give watch command time to start
		time.Sleep(500 * time.Millisecond)

		// Verify watch is still running (command hasn't completed)
		select {
		case err := <-watchDone:
			if err == nil {
				t.Fatal("Watch command completed too early - should be blocking")
			}
		default:
			// Good - command is still running
		}

		// Complete CI to unblock watch
		completeURL := "http://localhost:8081/test/ci/testorg/test-repo/" + prNumber + "/complete"
		resp, err := http.Post(completeURL, "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to complete CI: %v", err)
		}
		resp.Body.Close()

		// Now watch should complete
		select {
		case err := <-watchDone:
			if err != nil {
				t.Fatalf("Watch command failed after CI completion: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Watch command did not complete after CI completion")
		}

		t.Logf("Successfully verified blocking watch behavior")
	})

	// Test semver tool
	t.Run("Semver Tool Integration", func(t *testing.T) {
		semverPath := "/Users/danielgazineu/dev/workspace/tako/test/e2e/templates/java-bom-fanout/mock-semver.sh"

		// Test patch increment
		cmd := exec.Command(semverPath, "-i", "patch", "1.0.0")
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("Semver patch increment failed: %v", err)
		}

		result := string(bytes.TrimSpace(output))
		if result != "1.0.1" {
			t.Errorf("Expected semver patch increment to produce '1.0.1', got '%s'", result)
		}

		// Test minor increment
		cmd = exec.Command(semverPath, "-i", "minor", "1.0.5")
		output, err = cmd.Output()
		if err != nil {
			t.Fatalf("Semver minor increment failed: %v", err)
		}

		result = string(bytes.TrimSpace(output))
		if result != "1.1.0" {
			t.Errorf("Expected semver minor increment to produce '1.1.0', got '%s'", result)
		}

		// Test major increment
		cmd = exec.Command(semverPath, "-i", "major", "1.5.3")
		output, err = cmd.Output()
		if err != nil {
			t.Fatalf("Semver major increment failed: %v", err)
		}

		result = string(bytes.TrimSpace(output))
		if result != "2.0.0" {
			t.Errorf("Expected semver major increment to produce '2.0.0', got '%s'", result)
		}

		t.Logf("Successfully verified semver tool functionality")
	})
}

// TestConcurrentPRHandling tests the mock server's ability to handle concurrent PR operations
func TestConcurrentPRHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent PR test in short mode")
	}

	// Start mock GitHub server
	mockServer := NewMockGitHubServer()
	go func() {
		if err := mockServer.Start(8082); err != nil && err != http.ErrServerClosed {
			t.Errorf("Mock GitHub server failed: %v", err)
		}
	}()
	defer mockServer.Stop()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test concurrent PR creation
	concurrency := 5
	done := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			// Create PR
			prData := map[string]string{
				"title": "Concurrent PR " + string(rune('A'+id)),
				"body":  "Test concurrent PR creation",
				"head":  "feature-branch-" + string(rune('A'+id)),
				"base":  "main",
			}

			jsonData, _ := json.Marshal(prData)
			resp, err := http.Post("http://localhost:8082/repos/testorg/concurrent-test/pulls",
				"application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				done <- err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated {
				done <- err
				return
			}

			done <- nil
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < concurrency; i++ {
		if err := <-done; err != nil {
			t.Errorf("Concurrent PR creation failed: %v", err)
		}
	}

	// Verify all PRs were created
	prs := mockServer.ListPRs("testorg", "concurrent-test")
	if len(prs) != concurrency {
		t.Errorf("Expected %d PRs, got %d", concurrency, len(prs))
	}

	t.Logf("Successfully handled %d concurrent PR creations", concurrency)
}
