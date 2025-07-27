package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dangazineu/tako/internal/config"
)

func TestNewOrchestrator(t *testing.T) {
	workspaceRoot := "/tmp/workspace"
	cacheDir := "/tmp/cache"

	orchestrator := NewOrchestrator(workspaceRoot, cacheDir)

	if orchestrator.workspaceRoot != workspaceRoot {
		t.Errorf("Expected workspaceRoot %s, got %s", workspaceRoot, orchestrator.workspaceRoot)
	}
	if orchestrator.cacheDir != cacheDir {
		t.Errorf("Expected cacheDir %s, got %s", cacheDir, orchestrator.cacheDir)
	}
}

func TestExtractRepositoryName(t *testing.T) {
	tests := []struct {
		name     string
		repoPath string
		expected string
	}{
		{
			name:     "valid cache path",
			repoPath: "/home/user/.tako/cache/repos/owner/repo/main",
			expected: "owner/repo",
		},
		{
			name:     "valid cache path with branch",
			repoPath: "/home/user/.tako/cache/repos/myorg/myrepo/feature-branch",
			expected: "myorg/myrepo",
		},
		{
			name:     "invalid short path",
			repoPath: "/home/user/repos",
			expected: "",
		},
		{
			name:     "path without repos directory",
			repoPath: "/home/user/cache/owner/repo/main",
			expected: "",
		},
		{
			name:     "empty owner",
			repoPath: "/home/user/.tako/cache/repos//repo/main",
			expected: "",
		},
		{
			name:     "empty repo",
			repoPath: "/home/user/.tako/cache/repos/owner//main",
			expected: "",
		},
	}

	orchestrator := &Orchestrator{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := orchestrator.extractRepositoryName(tt.repoPath)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestSubscriptionMatches(t *testing.T) {
	orchestrator := &Orchestrator{}

	tests := []struct {
		name         string
		subscription config.Subscription
		eventType    string
		artifactRef  string
		eventPayload map[string]string
		expected     bool
	}{
		{
			name: "exact match",
			subscription: config.Subscription{
				Artifact: "owner/repo:artifact",
				Events:   []string{"library_built", "library_published"},
			},
			eventType:   "library_built",
			artifactRef: "owner/repo:artifact",
			expected:    true,
		},
		{
			name: "artifact mismatch",
			subscription: config.Subscription{
				Artifact: "owner/repo:artifact",
				Events:   []string{"library_built"},
			},
			eventType:   "library_built",
			artifactRef: "owner/other:artifact",
			expected:    false,
		},
		{
			name: "event type mismatch",
			subscription: config.Subscription{
				Artifact: "owner/repo:artifact",
				Events:   []string{"library_published"},
			},
			eventType:   "library_built",
			artifactRef: "owner/repo:artifact",
			expected:    false,
		},
		{
			name: "multiple events - match second",
			subscription: config.Subscription{
				Artifact: "owner/repo:artifact",
				Events:   []string{"library_built", "library_published"},
			},
			eventType:   "library_published",
			artifactRef: "owner/repo:artifact",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := orchestrator.subscriptionMatches(tt.subscription, tt.eventType, tt.artifactRef, tt.eventPayload)
			if result != tt.expected {
				t.Errorf("Expected %t, got %t", tt.expected, result)
			}
		})
	}
}

func TestValidateSchemaCompatibility(t *testing.T) {
	orchestrator := &Orchestrator{}

	tests := []struct {
		name                      string
		eventSchemaVersion        string
		subscriptionSchemaVersion string
		expected                  bool
	}{
		{
			name:                      "no subscription version requirement",
			eventSchemaVersion:        "1.0.0",
			subscriptionSchemaVersion: "",
			expected:                  true,
		},
		{
			name:                      "no event version, no subscription requirement",
			eventSchemaVersion:        "",
			subscriptionSchemaVersion: "",
			expected:                  true,
		},
		{
			name:                      "no event version, subscription has requirement",
			eventSchemaVersion:        "",
			subscriptionSchemaVersion: "1.0.0",
			expected:                  false,
		},
		{
			name:                      "exact version match",
			eventSchemaVersion:        "1.0.0",
			subscriptionSchemaVersion: "1.0.0",
			expected:                  true,
		},
		{
			name:                      "version mismatch",
			eventSchemaVersion:        "1.0.0",
			subscriptionSchemaVersion: "2.0.0",
			expected:                  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := orchestrator.ValidateSchemaCompatibility(tt.eventSchemaVersion, tt.subscriptionSchemaVersion)
			if result != tt.expected {
				t.Errorf("Expected %t, got %t", tt.expected, result)
			}
		})
	}
}

func TestEvaluateCELFilter(t *testing.T) {
	orchestrator := &Orchestrator{}

	// For now, this always returns true since CEL evaluation is not implemented
	tests := []struct {
		name         string
		filter       string
		eventContext map[string]interface{}
		expectedOk   bool
		expectedErr  bool
	}{
		{
			name:         "simple filter",
			filter:       "event.type == 'library_built'",
			eventContext: map[string]interface{}{"event": map[string]string{"type": "library_built"}},
			expectedOk:   true,
			expectedErr:  false,
		},
		{
			name:         "empty filter",
			filter:       "",
			eventContext: map[string]interface{}{},
			expectedOk:   true,
			expectedErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := orchestrator.EvaluateCELFilter(tt.filter, tt.eventContext)

			if tt.expectedErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if result != tt.expectedOk {
				t.Errorf("Expected %t, got %t", tt.expectedOk, result)
			}
		})
	}
}

func TestDiscoverRepositories(t *testing.T) {
	// Create temporary test environment
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")

	// Create test repository structure
	testRepos := []struct {
		path       string
		configName string
	}{
		{"repos/owner1/repo1/main", "tako.yml"},
		{"repos/owner1/repo2/main", "tako.yaml"},
		{"repos/owner2/repo3/feature", "tako.yml"},
		{"repos/invalid", "tako.yml"}, // Invalid structure
	}

	for _, repo := range testRepos {
		repoPath := filepath.Join(cacheDir, repo.path)
		err := os.MkdirAll(repoPath, 0755)
		if err != nil {
			t.Fatalf("Failed to create test directory %s: %v", repoPath, err)
		}

		configPath := filepath.Join(repoPath, repo.configName)
		err = os.WriteFile(configPath, []byte("version: v1\n"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test config %s: %v", configPath, err)
		}
	}

	orchestrator := NewOrchestrator(tempDir, cacheDir)
	ctx := context.Background()

	repositories, err := orchestrator.discoverRepositories(ctx)
	if err != nil {
		t.Fatalf("Failed to discover repositories: %v", err)
	}

	// Should find valid repositories (excluding invalid structure)
	expectedRepos := []string{"owner1/repo1", "owner1/repo2", "owner2/repo3"}
	if len(repositories) != len(expectedRepos) {
		t.Errorf("Expected %d repositories, got %d", len(expectedRepos), len(repositories))
	}

	// Verify repository names
	foundRepos := make(map[string]bool)
	for _, repoName := range repositories {
		foundRepos[repoName] = true
	}

	for _, expectedRepo := range expectedRepos {
		if !foundRepos[expectedRepo] {
			t.Errorf("Expected to find repository %s", expectedRepo)
		}
	}
}

func TestEvaluateRepositorySubscriptions(t *testing.T) {
	// Create temporary test environment
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "test-repo")
	err := os.MkdirAll(repoPath, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create test tako.yml with subscriptions
	configContent := `version: v1
subscriptions:
  - artifact: "owner/upstream:library"
    events: ["library_built", "library_published"]
    workflow: "update-deps"
  - artifact: "owner/other:service"
    events: ["service_deployed"]
    workflow: "integration-test"
workflows:
  update-deps:
    steps:
      - run: "echo updating deps"
  integration-test:
    steps:
      - run: "echo running tests"
`

	configPath := filepath.Join(repoPath, "tako.yml")
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	orchestrator := &Orchestrator{}
	ctx := context.Background()

	tests := []struct {
		name             string
		eventType        string
		artifactRef      string
		expectedMatches  int
		expectedWorkflow string
	}{
		{
			name:             "matching subscription",
			eventType:        "library_built",
			artifactRef:      "owner/upstream:library",
			expectedMatches:  1,
			expectedWorkflow: "update-deps",
		},
		{
			name:             "different event type",
			eventType:        "library_published",
			artifactRef:      "owner/upstream:library",
			expectedMatches:  1,
			expectedWorkflow: "update-deps",
		},
		{
			name:            "no matching artifact",
			eventType:       "library_built",
			artifactRef:     "owner/nonexistent:library",
			expectedMatches: 0,
		},
		{
			name:            "no matching event type",
			eventType:       "library_deleted",
			artifactRef:     "owner/upstream:library",
			expectedMatches: 0,
		},
		{
			name:             "different subscription match",
			eventType:        "service_deployed",
			artifactRef:      "owner/other:service",
			expectedMatches:  1,
			expectedWorkflow: "integration-test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := orchestrator.evaluateRepositorySubscriptions(
				ctx, repoPath, "owner/test-repo", tt.eventType, tt.artifactRef, map[string]string{})

			if err != nil {
				t.Fatalf("Failed to evaluate subscriptions: %v", err)
			}

			if len(matches) != tt.expectedMatches {
				t.Errorf("Expected %d matches, got %d", tt.expectedMatches, len(matches))
			}

			if tt.expectedMatches > 0 && len(matches) > 0 {
				if matches[0].Subscription.Workflow != tt.expectedWorkflow {
					t.Errorf("Expected workflow %s, got %s", tt.expectedWorkflow, matches[0].Subscription.Workflow)
				}
			}
		})
	}
}

func TestDiscoverSubscriptions_Integration(t *testing.T) {
	// Create temporary test environment
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")

	// Create multiple test repositories with subscriptions
	testRepos := []struct {
		owner         string
		repo          string
		branch        string
		subscriptions []string
	}{
		{
			owner:  "org1",
			repo:   "app1",
			branch: "main",
			subscriptions: []string{
				"owner/lib:core",
				"owner/lib:utils",
			},
		},
		{
			owner:  "org1",
			repo:   "app2",
			branch: "main",
			subscriptions: []string{
				"owner/lib:core",
			},
		},
		{
			owner:  "org2",
			repo:   "service",
			branch: "feature",
			subscriptions: []string{
				"owner/other:api",
			},
		},
	}

	for _, repo := range testRepos {
		repoPath := filepath.Join(cacheDir, "repos", repo.owner, repo.repo, repo.branch)
		err := os.MkdirAll(repoPath, 0755)
		if err != nil {
			t.Fatalf("Failed to create test directory %s: %v", repoPath, err)
		}

		// Create tako.yml with subscriptions
		configContent := "version: v1\nsubscriptions:\n"
		for _, sub := range repo.subscriptions {
			configContent += fmt.Sprintf(`  - artifact: "%s"
    events: ["library_built"]
    workflow: "update"
`, sub)
		}
		configContent += "workflows:\n  update:\n    steps:\n      - run: \"echo update\"\n"

		configPath := filepath.Join(repoPath, "tako.yml")
		err = os.WriteFile(configPath, []byte(configContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test config %s: %v", configPath, err)
		}
	}

	orchestrator := NewOrchestrator(tempDir, cacheDir)
	ctx := context.Background()

	// Test discovery for owner/lib:core (should match org1/app1 and org1/app2)
	matches, err := orchestrator.DiscoverSubscriptions(ctx, "library_built", "owner/lib:core", map[string]string{})
	if err != nil {
		t.Fatalf("Failed to discover subscriptions: %v", err)
	}

	if len(matches) != 2 {
		t.Errorf("Expected 2 matches for owner/lib:core, got %d", len(matches))
	}

	// Verify the matches are correct repositories
	foundRepos := make(map[string]bool)
	for _, match := range matches {
		foundRepos[match.RepositoryName] = true
	}

	expectedRepos := []string{"org1/app1", "org1/app2"}
	for _, expectedRepo := range expectedRepos {
		if !foundRepos[expectedRepo] {
			t.Errorf("Expected to find repository %s in matches", expectedRepo)
		}
	}

	// Test discovery for owner/other:api (should match org2/service)
	matches, err = orchestrator.DiscoverSubscriptions(ctx, "library_built", "owner/other:api", map[string]string{})
	if err != nil {
		t.Fatalf("Failed to discover subscriptions: %v", err)
	}

	if len(matches) != 1 {
		t.Errorf("Expected 1 match for owner/other:api, got %d", len(matches))
	}

	if len(matches) > 0 && matches[0].RepositoryName != "org2/service" {
		t.Errorf("Expected match for org2/service, got %s", matches[0].RepositoryName)
	}

	// Test discovery for non-existent artifact (should match none)
	matches, err = orchestrator.DiscoverSubscriptions(ctx, "library_built", "owner/nonexistent:artifact", map[string]string{})
	if err != nil {
		t.Fatalf("Failed to discover subscriptions: %v", err)
	}

	if len(matches) != 0 {
		t.Errorf("Expected 0 matches for non-existent artifact, got %d", len(matches))
	}
}
