package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dangazineu/tako/internal/config"
)

func TestNewDiscoveryManager(t *testing.T) {
	cacheDir := "/test/cache"
	dm := NewDiscoveryManager(cacheDir)

	if dm.cacheDir != cacheDir {
		t.Errorf("Expected cache directory %s, got %s", cacheDir, dm.cacheDir)
	}
}

func TestDiscoveryManager_FindSubscribers(t *testing.T) {
	// Create temporary directory structure for testing
	tempDir := t.TempDir()
	cacheDir := tempDir

	// Create test repository structure
	testRepo1Path := filepath.Join(cacheDir, "repos", "test-org", "repo1", "main")
	testRepo2Path := filepath.Join(cacheDir, "repos", "test-org", "repo2", "main")

	if err := os.MkdirAll(testRepo1Path, 0755); err != nil {
		t.Fatalf("Failed to create test repo1 directory: %v", err)
	}
	if err := os.MkdirAll(testRepo2Path, 0755); err != nil {
		t.Fatalf("Failed to create test repo2 directory: %v", err)
	}

	// Create tako.yml files with subscriptions
	takoYml1 := `version: "1.0"
workflows:
  update:
    steps:
      - run: echo "update"
  other:
    steps:
      - run: echo "other"
subscriptions:
  - artifact: "test-org/library:lib"
    events: ["library_built", "library_updated"]
    workflow: "update"
  - artifact: "other-org/other:other"
    events: ["other_event"]
    workflow: "other"
`

	takoYml2 := `version: "1.0"
workflows:
  build:
    steps:
      - run: echo "build"
subscriptions:
  - artifact: "test-org/library:lib"
    events: ["library_built"]
    workflow: "build"
`

	if err := os.WriteFile(filepath.Join(testRepo1Path, "tako.yml"), []byte(takoYml1), 0644); err != nil {
		t.Fatalf("Failed to write tako.yml for repo1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testRepo2Path, "tako.yml"), []byte(takoYml2), 0644); err != nil {
		t.Fatalf("Failed to write tako.yml for repo2: %v", err)
	}

	dm := NewDiscoveryManager(cacheDir)

	tests := []struct {
		name        string
		artifact    string
		eventType   string
		wantCount   int
		wantRepos   []string
		expectError bool
	}{
		{
			name:      "find subscribers for library_built",
			artifact:  "test-org/library:lib",
			eventType: "library_built",
			wantCount: 2,
			wantRepos: []string{"test-org/repo1", "test-org/repo2"},
		},
		{
			name:      "find subscribers for library_updated",
			artifact:  "test-org/library:lib",
			eventType: "library_updated",
			wantCount: 1,
			wantRepos: []string{"test-org/repo1"},
		},
		{
			name:      "find subscribers for non-existent event",
			artifact:  "test-org/library:lib",
			eventType: "non_existent_event",
			wantCount: 0,
			wantRepos: []string{},
		},
		{
			name:      "find subscribers for non-existent artifact",
			artifact:  "non-existent/artifact:test",
			eventType: "library_built",
			wantCount: 0,
			wantRepos: []string{},
		},
		{
			name:        "empty artifact should error",
			artifact:    "",
			eventType:   "library_built",
			expectError: true,
		},
		{
			name:        "empty event type should error",
			artifact:    "test-org/library:lib",
			eventType:   "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := dm.FindSubscribers(tt.artifact, tt.eventType)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(matches) != tt.wantCount {
				t.Errorf("Expected %d matches, got %d", tt.wantCount, len(matches))
				return
			}

			// Check that repositories are sorted alphabetically
			for i, match := range matches {
				expectedRepo := tt.wantRepos[i]
				if match.Repository != expectedRepo {
					t.Errorf("Expected repository %s at index %d, got %s", expectedRepo, i, match.Repository)
				}
			}

			// Verify subscription details for first match if any
			if len(matches) > 0 {
				firstMatch := matches[0]
				if firstMatch.Subscription.Artifact != tt.artifact {
					t.Errorf("Expected subscription artifact %s, got %s", tt.artifact, firstMatch.Subscription.Artifact)
				}

				// Check that the event type is in the subscription's events
				found := false
				for _, event := range firstMatch.Subscription.Events {
					if event == tt.eventType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Event type %s not found in subscription events", tt.eventType)
				}
			}
		})
	}
}

func TestDiscoveryManager_LoadSubscriptions(t *testing.T) {
	tempDir := t.TempDir()
	dm := NewDiscoveryManager(tempDir)

	tests := []struct {
		name        string
		takoYml     string
		wantCount   int
		expectError bool
	}{
		{
			name: "valid tako.yml with subscriptions",
			takoYml: `version: "1.0"
workflows:
  test:
    steps:
      - run: echo "test"
subscriptions:
  - artifact: "owner/repo:artifact"
    events: ["event1", "event2"]
    workflow: "test"
`,
			wantCount: 1,
		},
		{
			name: "valid tako.yml without subscriptions",
			takoYml: `version: "1.0"
workflows:
  test:
    steps:
      - run: echo "test"
`,
			wantCount: 0,
		},
		{
			name: "empty subscriptions array",
			takoYml: `version: "1.0"
subscriptions: []
`,
			wantCount: 0,
		},
		{
			name:        "invalid yaml",
			takoYml:     "invalid: yaml: content: [",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test directory
			testRepoPath := filepath.Join(tempDir, "test-repo")
			if err := os.MkdirAll(testRepoPath, 0755); err != nil {
				t.Fatalf("Failed to create test directory: %v", err)
			}

			// Write tako.yml file
			takoYmlPath := filepath.Join(testRepoPath, "tako.yml")
			if err := os.WriteFile(takoYmlPath, []byte(tt.takoYml), 0644); err != nil {
				t.Fatalf("Failed to write tako.yml: %v", err)
			}

			subscriptions, err := dm.LoadSubscriptions(testRepoPath)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(subscriptions) != tt.wantCount {
				t.Errorf("Expected %d subscriptions, got %d", tt.wantCount, len(subscriptions))
			}
		})
	}
}

func TestDiscoveryManager_LoadSubscriptions_NoFile(t *testing.T) {
	tempDir := t.TempDir()
	dm := NewDiscoveryManager(tempDir)

	// Create directory without tako.yml
	testRepoPath := filepath.Join(tempDir, "empty-repo")
	if err := os.MkdirAll(testRepoPath, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	subscriptions, err := dm.LoadSubscriptions(testRepoPath)
	if err != nil {
		t.Errorf("Expected no error for missing tako.yml, got: %v", err)
	}

	if len(subscriptions) != 0 {
		t.Errorf("Expected 0 subscriptions for missing tako.yml, got %d", len(subscriptions))
	}
}

func TestDiscoveryManager_GetRepositoryPath(t *testing.T) {
	cacheDir := "/test/cache"
	dm := NewDiscoveryManager(cacheDir)

	tests := []struct {
		name   string
		owner  string
		repo   string
		branch string
		want   string
	}{
		{
			name:   "with explicit branch",
			owner:  "owner",
			repo:   "repo",
			branch: "feature",
			want:   "/test/cache/repos/owner/repo/feature",
		},
		{
			name:   "with empty branch defaults to main",
			owner:  "owner",
			repo:   "repo",
			branch: "",
			want:   "/test/cache/repos/owner/repo/main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dm.GetRepositoryPath(tt.owner, tt.repo, tt.branch)
			if got != tt.want {
				t.Errorf("GetRepositoryPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiscoveryManager_ScanRepositories(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := tempDir
	dm := NewDiscoveryManager(cacheDir)

	// Test with empty cache directory
	repos, err := dm.ScanRepositories()
	if err != nil {
		t.Errorf("Unexpected error with empty cache: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("Expected 0 repositories in empty cache, got %d", len(repos))
	}

	// Create test repository structure
	testRepos := []string{
		"owner1/repo1",
		"owner1/repo2",
		"owner2/repo1",
	}

	for _, repo := range testRepos {
		parts := strings.Split(repo, "/")
		repoPath := filepath.Join(cacheDir, "repos", parts[0], parts[1], "main")
		if err := os.MkdirAll(repoPath, 0755); err != nil {
			t.Fatalf("Failed to create test repo %s: %v", repo, err)
		}
	}

	// Test scanning populated cache
	repos, err = dm.ScanRepositories()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(repos) != len(testRepos) {
		t.Errorf("Expected %d repositories, got %d", len(testRepos), len(repos))
	}

	// Verify repositories are sorted
	expectedRepos := []string{"owner1/repo1", "owner1/repo2", "owner2/repo1"}
	for i, expectedRepo := range expectedRepos {
		if i >= len(repos) || repos[i] != expectedRepo {
			t.Errorf("Expected repository %s at index %d, got %s", expectedRepo, i, repos[i])
		}
	}
}

func TestDiscoveryManager_matchesArtifactAndEvent(t *testing.T) {
	dm := NewDiscoveryManager("/test")

	subscription := config.Subscription{
		Artifact: "owner/repo:artifact",
		Events:   []string{"event1", "event2", "event3"},
	}

	tests := []struct {
		name      string
		artifact  string
		eventType string
		want      bool
	}{
		{
			name:      "exact artifact and event match",
			artifact:  "owner/repo:artifact",
			eventType: "event1",
			want:      true,
		},
		{
			name:      "exact artifact, different event",
			artifact:  "owner/repo:artifact",
			eventType: "event2",
			want:      true,
		},
		{
			name:      "different artifact, same event",
			artifact:  "different/repo:artifact",
			eventType: "event1",
			want:      false,
		},
		{
			name:      "same artifact, non-existent event",
			artifact:  "owner/repo:artifact",
			eventType: "non_existent",
			want:      false,
		},
		{
			name:      "different artifact, non-existent event",
			artifact:  "different/repo:artifact",
			eventType: "non_existent",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dm.matchesArtifactAndEvent(subscription, tt.artifact, tt.eventType)
			if got != tt.want {
				t.Errorf("matchesArtifactAndEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}
