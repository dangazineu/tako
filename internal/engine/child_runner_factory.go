package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ChildRunnerFactory creates isolated Runner instances for child workflow execution.
// It ensures proper workspace isolation while sharing the cache directory for efficiency.
type ChildRunnerFactory struct {
	// Parent configuration
	parentWorkspaceRoot string
	cacheDir            string
	maxConcurrentRepos  int
	debug               bool
	environment         []string

	// Cache locking to prevent race conditions
	cacheLockManager *LockManager

	// Synchronization
	mu sync.RWMutex
}

// NewChildRunnerFactory creates a new factory for child Runner instances.
// It uses the parent's workspace root to create isolated child workspaces
// and shares the cache directory to avoid re-downloading repositories.
func NewChildRunnerFactory(parentWorkspaceRoot, cacheDir string, maxConcurrentRepos int, debug bool, environment []string) (*ChildRunnerFactory, error) {
	if parentWorkspaceRoot == "" {
		return nil, fmt.Errorf("parent workspace root is required")
	}
	if cacheDir == "" {
		return nil, fmt.Errorf("cache directory is required")
	}

	// Create children directory under parent workspace
	childrenDir := filepath.Join(parentWorkspaceRoot, "children")
	if err := os.MkdirAll(childrenDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create children directory: %w", err)
	}

	// Initialize cache lock manager for preventing race conditions
	cacheLockManager, err := NewLockManager(filepath.Join(cacheDir, "locks"))
	if err != nil {
		return nil, fmt.Errorf("failed to create cache lock manager: %w", err)
	}

	return &ChildRunnerFactory{
		parentWorkspaceRoot: parentWorkspaceRoot,
		cacheDir:            cacheDir,
		maxConcurrentRepos:  maxConcurrentRepos,
		debug:               debug,
		environment:         environment,
		cacheLockManager:    cacheLockManager,
	}, nil
}

// CreateChildRunner creates a new isolated Runner instance for child workflow execution.
// Each child gets its own workspace directory but shares the cache directory.
// Returns the new Runner and its unique workspace path.
func (f *ChildRunnerFactory) CreateChildRunner() (*Runner, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Generate unique run ID for this child
	childRunID := GenerateRunID()

	// Create isolated workspace for this child
	childWorkspace := filepath.Join(f.parentWorkspaceRoot, "children", childRunID)

	if err := os.MkdirAll(childWorkspace, 0755); err != nil {
		return nil, "", fmt.Errorf("failed to create child workspace %s: %w", childWorkspace, err)
	}

	// Create RunnerOptions for the child with isolated workspace
	opts := RunnerOptions{
		WorkspaceRoot:      childWorkspace,
		CacheDir:           f.cacheDir, // Shared cache directory
		MaxConcurrentRepos: f.maxConcurrentRepos,
		DryRun:             false, // Child executions should not be dry run
		Debug:              f.debug,
		NoCache:            false, // Use cache for efficiency
		Environment:        f.environment,
	}

	// Create the child Runner instance
	childRunner, err := NewRunner(opts)
	if err != nil {
		// Clean up the workspace if Runner creation fails
		os.RemoveAll(childWorkspace)
		return nil, "", fmt.Errorf("failed to create child runner: %w", err)
	}

	return childRunner, childWorkspace, nil
}

// AcquireCacheLock acquires a lock for cache operations to prevent race conditions.
// The lock is scoped to a specific repository to allow concurrent access to different repos.
func (f *ChildRunnerFactory) AcquireCacheLock(ctx context.Context, runID, repository string, lockType LockType) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Use the existing LockManager interface which requires context and runID
	return f.cacheLockManager.AcquireLock(ctx, runID, repository, lockType)
}

// ReleaseCacheLock releases a previously acquired cache lock.
func (f *ChildRunnerFactory) ReleaseCacheLock(runID, repository string, lockType LockType) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Use the existing LockManager interface
	return f.cacheLockManager.ReleaseLock(runID, repository, lockType)
}

// Close cleans up the factory resources.
func (f *ChildRunnerFactory) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cacheLockManager != nil {
		return f.cacheLockManager.Close()
	}
	return nil
}

// GetChildrenDirectory returns the path to the children directory.
// This is useful for cleanup operations and testing.
func (f *ChildRunnerFactory) GetChildrenDirectory() string {
	return filepath.Join(f.parentWorkspaceRoot, "children")
}
