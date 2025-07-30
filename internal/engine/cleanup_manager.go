package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CleanupManager handles cleanup of child workflow workspaces and orphaned resources.
type CleanupManager struct {
	workspaceRoot string
	maxAge        time.Duration
	debug         bool
}

// NewCleanupManager creates a new cleanup manager.
func NewCleanupManager(workspaceRoot string, maxAge time.Duration, debug bool) *CleanupManager {
	if maxAge == 0 {
		maxAge = 24 * time.Hour // Default: cleanup workspaces older than 24 hours
	}

	return &CleanupManager{
		workspaceRoot: workspaceRoot,
		maxAge:        maxAge,
		debug:         debug,
	}
}

// CleanupOrphanedWorkspaces removes child workflow workspaces that are older than maxAge
// and don't have active processes. This is an idempotent operation.
func (cm *CleanupManager) CleanupOrphanedWorkspaces() error {
	if cm.debug {
		fmt.Printf("Starting cleanup of orphaned workspaces in %s (max age: %v)\n", cm.workspaceRoot, cm.maxAge)
	}

	// Look for "children" directories within workspace directories
	err := filepath.Walk(cm.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip non-directories
		if !info.IsDir() {
			return nil
		}

		// Look for child workspace directories (pattern: workspace_root/*/children/run-id)
		// Skip the "children" directory itself, only process its subdirectories
		if strings.Contains(path, "/children/") && !strings.HasSuffix(path, "/children") {
			return cm.cleanupChildWorkspace(path, info)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk workspace directory: %v", err)
	}

	if cm.debug {
		fmt.Printf("Cleanup completed\n")
	}

	return nil
}

// cleanupChildWorkspace handles cleanup of individual child workspaces.
func (cm *CleanupManager) cleanupChildWorkspace(path string, info os.FileInfo) error {
	// This should only be called for actual child workspace directories (not the "children" dir itself)
	if !strings.Contains(path, "/children/") || strings.HasSuffix(path, "/children") {
		return nil
	}

	// Check age
	if time.Since(info.ModTime()) < cm.maxAge {
		if cm.debug {
			fmt.Printf("Skipping %s (too recent: %v)\n", path, time.Since(info.ModTime()))
		}
		return nil
	}

	// Check if there are any active processes (basic check)
	if cm.hasActiveProcesses(path) {
		if cm.debug {
			fmt.Printf("Skipping %s (has active processes)\n", path)
		}
		return nil
	}

	// Safe to remove
	if cm.debug {
		fmt.Printf("Removing orphaned workspace: %s\n", path)
	}

	err := os.RemoveAll(path)
	if err != nil {
		return fmt.Errorf("failed to remove orphaned workspace %s: %v", path, err)
	}

	// Return SkipDir to tell filepath.Walk to skip this directory and its contents
	return filepath.SkipDir
}

// hasActiveProcesses performs a basic check for active processes in the workspace.
// This is a simple implementation that checks for common lock files.
func (cm *CleanupManager) hasActiveProcesses(workspacePath string) bool {
	// Check for common lock files that indicate active processes
	lockFiles := []string{
		".tako-lock",
		".git/index.lock",
		"go.sum.lock",
		"package-lock.json.lock",
	}

	for _, lockFile := range lockFiles {
		lockPath := filepath.Join(workspacePath, lockFile)
		if _, err := os.Stat(lockPath); err == nil {
			return true
		}
	}

	return false
}

// CleanupChildWorkspace removes a specific child workspace directory.
// This is called when a child workflow completes successfully.
func (cm *CleanupManager) CleanupChildWorkspace(runID string) error {
	if runID == "" {
		return fmt.Errorf("runID cannot be empty")
	}

	// Find the child workspace directory
	var workspaceToRemove string
	err := filepath.Walk(cm.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && strings.HasSuffix(path, "/children/"+runID) {
			workspaceToRemove = path
			return filepath.SkipDir // Found it, no need to continue
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to find child workspace for runID %s: %v", runID, err)
	}

	if workspaceToRemove == "" {
		// Workspace doesn't exist (already cleaned up or never created)
		if cm.debug {
			fmt.Printf("Child workspace for runID %s not found (already cleaned up)\n", runID)
		}
		return nil
	}

	if cm.debug {
		fmt.Printf("Cleaning up child workspace: %s\n", workspaceToRemove)
	}

	err = os.RemoveAll(workspaceToRemove)
	if err != nil {
		return fmt.Errorf("failed to remove child workspace %s: %v", workspaceToRemove, err)
	}

	return nil
}

// GetOrphanedWorkspaceStats returns statistics about orphaned workspaces.
func (cm *CleanupManager) GetOrphanedWorkspaceStats() (int, int64, error) {
	var orphanedCount int
	var totalSize int64

	err := filepath.Walk(cm.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			return nil
		}

		// Look for child workspace directories that are candidates for cleanup
		if strings.Contains(path, "/children/") && time.Since(info.ModTime()) > cm.maxAge && !cm.hasActiveProcesses(path) {
			orphanedCount++

			// Calculate directory size
			size, sizeErr := cm.calculateDirectorySize(path)
			if sizeErr == nil {
				totalSize += size
			}
		}

		return nil
	})

	if err != nil {
		return 0, 0, fmt.Errorf("failed to calculate orphaned workspace stats: %v", err)
	}

	return orphanedCount, totalSize, nil
}

// calculateDirectorySize calculates the total size of a directory.
func (cm *CleanupManager) calculateDirectorySize(dirPath string) (int64, error) {
	var size int64

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			size += info.Size()
		}

		return nil
	})

	return size, err
}
