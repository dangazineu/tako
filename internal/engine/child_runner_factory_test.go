package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewChildRunnerFactory(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name                   string
		parentWorkspaceRoot    string
		cacheDir               string
		maxConcurrentRepos     int
		debug                  bool
		environment            []string
		expectError            bool
		expectedErrorSubstring string
	}{
		{
			name:                "valid factory creation",
			parentWorkspaceRoot: filepath.Join(tempDir, "parent"),
			cacheDir:            filepath.Join(tempDir, "cache"),
			maxConcurrentRepos:  5,
			debug:               true,
			environment:         []string{"TEST=1"},
			expectError:         false,
		},
		{
			name:                   "empty parent workspace root",
			parentWorkspaceRoot:    "",
			cacheDir:               filepath.Join(tempDir, "cache"),
			maxConcurrentRepos:     5,
			debug:                  false,
			environment:            []string{},
			expectError:            true,
			expectedErrorSubstring: "parent workspace root is required",
		},
		{
			name:                   "empty cache directory",
			parentWorkspaceRoot:    filepath.Join(tempDir, "parent"),
			cacheDir:               "",
			maxConcurrentRepos:     5,
			debug:                  false,
			environment:            []string{},
			expectError:            true,
			expectedErrorSubstring: "cache directory is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, err := NewChildRunnerFactory(
				tt.parentWorkspaceRoot,
				tt.cacheDir,
				tt.maxConcurrentRepos,
				tt.debug,
				tt.environment,
			)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				} else if tt.expectedErrorSubstring != "" && !contains(err.Error(), tt.expectedErrorSubstring) {
					t.Errorf("Expected error to contain '%s', but got: %v", tt.expectedErrorSubstring, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if factory == nil {
				t.Error("Factory should not be nil")
				return
			}

			// Verify that children directory was created
			childrenDir := filepath.Join(tt.parentWorkspaceRoot, "children")
			if _, err := os.Stat(childrenDir); os.IsNotExist(err) {
				t.Errorf("Children directory was not created: %s", childrenDir)
			}

			// Verify factory fields
			if factory.parentWorkspaceRoot != tt.parentWorkspaceRoot {
				t.Errorf("Expected parentWorkspaceRoot %s, got %s", tt.parentWorkspaceRoot, factory.parentWorkspaceRoot)
			}
			if factory.cacheDir != tt.cacheDir {
				t.Errorf("Expected cacheDir %s, got %s", tt.cacheDir, factory.cacheDir)
			}
			if factory.maxConcurrentRepos != tt.maxConcurrentRepos {
				t.Errorf("Expected maxConcurrentRepos %d, got %d", tt.maxConcurrentRepos, factory.maxConcurrentRepos)
			}
			if factory.debug != tt.debug {
				t.Errorf("Expected debug %v, got %v", tt.debug, factory.debug)
			}

			// Clean up
			factory.Close()
		})
	}
}

func TestChildRunnerFactory_CreateChildRunner(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	// Create first child runner
	child1, workspace1, err := factory.CreateChildRunner()
	if err != nil {
		t.Fatalf("Failed to create first child runner: %v", err)
	}
	defer child1.Close()

	// Create second child runner
	child2, workspace2, err := factory.CreateChildRunner()
	if err != nil {
		t.Fatalf("Failed to create second child runner: %v", err)
	}
	defer child2.Close()

	// Verify workspace isolation
	if workspace1 == workspace2 {
		t.Error("Child workspaces should be different")
	}

	// Verify workspaces exist and are under parent/children
	expectedPrefix := filepath.Join(parentWorkspace, "children")
	if !hasPrefix(workspace1, expectedPrefix) {
		t.Errorf("Child workspace1 %s should be under %s", workspace1, expectedPrefix)
	}
	if !hasPrefix(workspace2, expectedPrefix) {
		t.Errorf("Child workspace2 %s should be under %s", workspace2, expectedPrefix)
	}

	// Verify workspaces actually exist on filesystem
	if _, err := os.Stat(workspace1); os.IsNotExist(err) {
		t.Errorf("Child workspace1 should exist: %s", workspace1)
	}
	if _, err := os.Stat(workspace2); os.IsNotExist(err) {
		t.Errorf("Child workspace2 should exist: %s", workspace2)
	}

	// Verify runners are properly configured
	if child1 == nil || child2 == nil {
		t.Error("Child runners should not be nil")
	}

	// Verify that children use the shared cache directory
	// Note: We can't directly access the cacheDir from Runner,
	// but we can verify it through successful factory creation
}

func TestChildRunnerFactory_WorkspaceIsolation(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	// Create multiple child runners
	var children []*Runner
	var workspaces []string
	numChildren := 10

	for i := 0; i < numChildren; i++ {
		child, workspace, err := factory.CreateChildRunner()
		if err != nil {
			t.Fatalf("Failed to create child runner %d: %v", i, err)
		}
		children = append(children, child)
		workspaces = append(workspaces, workspace)
	}

	// Clean up all children
	defer func() {
		for _, child := range children {
			child.Close()
		}
	}()

	// Verify all workspaces are unique
	workspaceSet := make(map[string]bool)
	for i, workspace := range workspaces {
		if workspaceSet[workspace] {
			t.Errorf("Workspace %s is not unique (child %d)", workspace, i)
		}
		workspaceSet[workspace] = true
	}

	// Verify all workspaces exist
	for i, workspace := range workspaces {
		if _, err := os.Stat(workspace); os.IsNotExist(err) {
			t.Errorf("Child workspace %d should exist: %s", i, workspace)
		}
	}
}

func TestChildRunnerFactory_ConcurrentCreation(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	// Create children concurrently
	numGoroutines := 20
	results := make(chan struct {
		runner    *Runner
		workspace string
		err       error
	}, numGoroutines)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runner, workspace, err := factory.CreateChildRunner()
			results <- struct {
				runner    *Runner
				workspace string
				err       error
			}{runner, workspace, err}
		}()
	}

	wg.Wait()
	close(results)

	// Collect results
	var children []*Runner
	var workspaces []string
	var errors []error

	for result := range results {
		if result.err != nil {
			errors = append(errors, result.err)
		} else {
			children = append(children, result.runner)
			workspaces = append(workspaces, result.workspace)
		}
	}

	// Clean up children
	defer func() {
		for _, child := range children {
			child.Close()
		}
	}()

	// Verify no errors occurred
	if len(errors) > 0 {
		t.Errorf("Expected no errors, but got %d errors. First error: %v", len(errors), errors[0])
	}

	// Verify all workspaces are unique
	workspaceSet := make(map[string]bool)
	for _, workspace := range workspaces {
		if workspaceSet[workspace] {
			t.Errorf("Workspace %s is not unique in concurrent test", workspace)
		}
		workspaceSet[workspace] = true
	}

	// Verify we got the expected number of successful creations
	if len(children) != numGoroutines {
		t.Errorf("Expected %d children, got %d", numGoroutines, len(children))
	}
}

func TestChildRunnerFactory_CacheLocking(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	ctx := context.Background()
	runID := "test-run-id"
	repository := "test-org/test-repo"

	// Test read lock acquisition and release
	err = factory.AcquireCacheLock(ctx, runID, repository, LockTypeRead)
	if err != nil {
		t.Errorf("Failed to acquire read lock: %v", err)
	}

	err = factory.ReleaseCacheLock(runID, repository, LockTypeRead)
	if err != nil {
		t.Errorf("Failed to release read lock: %v", err)
	}

	// Test write lock acquisition and release
	err = factory.AcquireCacheLock(ctx, runID, repository, LockTypeWrite)
	if err != nil {
		t.Errorf("Failed to acquire write lock: %v", err)
	}

	err = factory.ReleaseCacheLock(runID, repository, LockTypeWrite)
	if err != nil {
		t.Errorf("Failed to release write lock: %v", err)
	}
}

func TestChildRunnerFactory_ConcurrentCacheLocking(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	ctx := context.Background()
	numGoroutines := 10

	// Test concurrent access to different repositories (should be allowed)
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			runID := fmt.Sprintf("test-run-id-%d", goroutineID)
			repository := fmt.Sprintf("test-org/test-repo-%d", goroutineID)

			// Acquire read lock on different repository per goroutine
			if err := factory.AcquireCacheLock(ctx, runID, repository, LockTypeRead); err != nil {
				errors <- err
				return
			}

			// Hold lock briefly
			time.Sleep(10 * time.Millisecond)

			// Release read lock
			if err := factory.ReleaseCacheLock(runID, repository, LockTypeRead); err != nil {
				errors <- err
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var errorList []error
	for err := range errors {
		errorList = append(errorList, err)
	}

	if len(errorList) > 0 {
		t.Errorf("Expected no errors in concurrent different repository lock test, got %d errors. First: %v", len(errorList), errorList[0])
	}
}

func TestChildRunnerFactory_GetChildrenDirectory(t *testing.T) {
	tempDir := t.TempDir()
	parentWorkspace := filepath.Join(tempDir, "parent")
	cacheDir := filepath.Join(tempDir, "cache")

	factory, err := NewChildRunnerFactory(parentWorkspace, cacheDir, 5, false, []string{})
	if err != nil {
		t.Fatalf("Failed to create factory: %v", err)
	}
	defer factory.Close()

	expectedDir := filepath.Join(parentWorkspace, "children")
	actualDir := factory.GetChildrenDirectory()

	if actualDir != expectedDir {
		t.Errorf("Expected children directory %s, got %s", expectedDir, actualDir)
	}
}

// Helper functions for testing

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr ||
		len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
