package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCleanupManager(t *testing.T) {
	tempDir := t.TempDir()

	// Test with default maxAge
	cm1 := NewCleanupManager(tempDir, 0, false)
	if cm1.workspaceRoot != tempDir {
		t.Errorf("Expected workspace root %s, got %s", tempDir, cm1.workspaceRoot)
	}
	if cm1.maxAge != 24*time.Hour {
		t.Errorf("Expected default maxAge 24h, got %v", cm1.maxAge)
	}

	// Test with custom maxAge
	customMaxAge := 2 * time.Hour
	cm2 := NewCleanupManager(tempDir, customMaxAge, true)
	if cm2.maxAge != customMaxAge {
		t.Errorf("Expected maxAge %v, got %v", customMaxAge, cm2.maxAge)
	}
	if !cm2.debug {
		t.Errorf("Expected debug true, got false")
	}
}

func TestCleanupManager_CleanupOrphanedWorkspaces(t *testing.T) {
	tempDir := t.TempDir()
	// Use a very short maxAge to ensure cleanup happens in tests
	cm := NewCleanupManager(tempDir, 10*time.Millisecond, true)

	// Create some test workspace structures
	oldWorkspace := filepath.Join(tempDir, "project1", "children", "old-run-id")
	newWorkspace := filepath.Join(tempDir, "project2", "children", "new-run-id")

	if err := os.MkdirAll(oldWorkspace, 0755); err != nil {
		t.Fatalf("Failed to create old workspace: %v", err)
	}

	// Add some content to the old workspace
	if err := os.WriteFile(filepath.Join(oldWorkspace, "test.txt"), []byte("old"), 0644); err != nil {
		t.Fatalf("Failed to write to old workspace: %v", err)
	}

	// Wait for the directory to be old enough
	time.Sleep(20 * time.Millisecond)

	// Create new workspace after waiting
	if err := os.MkdirAll(newWorkspace, 0755); err != nil {
		t.Fatalf("Failed to create new workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newWorkspace, "test.txt"), []byte("new"), 0644); err != nil {
		t.Fatalf("Failed to write to new workspace: %v", err)
	}

	// Run cleanup
	err := cm.CleanupOrphanedWorkspaces()
	if err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}

	// Old workspace should be removed
	if _, err := os.Stat(oldWorkspace); !os.IsNotExist(err) {
		t.Errorf("Old workspace should have been removed")
	}

	// New workspace should still exist
	if _, err := os.Stat(newWorkspace); err != nil {
		t.Errorf("New workspace should still exist: %v", err)
	}
}

func TestCleanupManager_CleanupChildWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	cm := NewCleanupManager(tempDir, 1*time.Hour, false)

	// Create a specific child workspace
	runID := "test-run-123"
	workspacePath := filepath.Join(tempDir, "project", "children", runID)

	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	// Add some content
	if err := os.WriteFile(filepath.Join(workspacePath, "test.txt"), []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Cleanup the specific workspace
	err := cm.CleanupChildWorkspace(runID)
	if err != nil {
		t.Errorf("Failed to cleanup child workspace: %v", err)
	}

	// Workspace should be removed
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Errorf("Workspace should have been removed")
	}

	// Cleanup non-existent workspace should not error
	err = cm.CleanupChildWorkspace("non-existent-run")
	if err != nil {
		t.Errorf("Cleanup of non-existent workspace should not error: %v", err)
	}
}

func TestCleanupManager_GetOrphanedWorkspaceStats(t *testing.T) {
	tempDir := t.TempDir()
	// Use short maxAge for testing
	cm := NewCleanupManager(tempDir, 10*time.Millisecond, false)

	// Create orphaned workspace first
	orphanedPath := filepath.Join(tempDir, "project1", "children", "orphaned-run")
	if err := os.MkdirAll(orphanedPath, 0755); err != nil {
		t.Fatalf("Failed to create orphaned workspace: %v", err)
	}

	// Add some content
	testContent := []byte("test content")
	if err := os.WriteFile(filepath.Join(orphanedPath, "test.txt"), testContent, 0644); err != nil {
		t.Fatalf("Failed to write to orphaned workspace: %v", err)
	}

	// Wait for workspace to be old enough
	time.Sleep(20 * time.Millisecond)

	// Create recent workspace
	recentPath := filepath.Join(tempDir, "project2", "children", "recent-run")
	if err := os.MkdirAll(recentPath, 0755); err != nil {
		t.Fatalf("Failed to create recent workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(recentPath, "test.txt"), testContent, 0644); err != nil {
		t.Fatalf("Failed to write to recent workspace: %v", err)
	}

	// Get stats
	count, size, err := cm.GetOrphanedWorkspaceStats()
	if err != nil {
		t.Errorf("Failed to get stats: %v", err)
	}

	// Should find 1 orphaned workspace
	if count != 1 {
		t.Errorf("Expected 1 orphaned workspace, got %d", count)
	}

	// Size should be greater than 0
	if size == 0 {
		t.Errorf("Expected size > 0, got %d", size)
	}
}

func TestCleanupManager_hasActiveProcesses(t *testing.T) {
	tempDir := t.TempDir()
	cm := NewCleanupManager(tempDir, 1*time.Hour, false)

	// Create workspace without lock files
	workspacePath := filepath.Join(tempDir, "workspace")
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	// Should not have active processes
	if cm.hasActiveProcesses(workspacePath) {
		t.Errorf("Empty workspace should not have active processes")
	}

	// Add a lock file
	lockFile := filepath.Join(workspacePath, ".tako-lock")
	if err := os.WriteFile(lockFile, []byte("lock"), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	// Should now have active processes
	if !cm.hasActiveProcesses(workspacePath) {
		t.Errorf("Workspace with lock file should have active processes")
	}
}
