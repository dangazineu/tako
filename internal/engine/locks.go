package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LockType defines the type of lock being held.
type LockType string

const (
	LockTypeRead  LockType = "read"
	LockTypeWrite LockType = "write"
)

// LockInfo contains information about a held lock.
type LockInfo struct {
	RunID      string    `json:"run_id"`
	Repository string    `json:"repository"`
	Type       LockType  `json:"type"`
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	ProcessID  int       `json:"process_id"`
}

// LockManager provides fine-grained repository locking with deadlock detection.
type LockManager struct {
	lockDir string
	locks   map[string]*LockInfo
	mu      sync.RWMutex

	// Lock timeout configuration
	defaultTimeout time.Duration
	maxTimeout     time.Duration
}

// NewLockManager creates a new lock manager.
func NewLockManager(lockDir string) (*LockManager, error) {
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %v", err)
	}

	lm := &LockManager{
		lockDir:        lockDir,
		locks:          make(map[string]*LockInfo),
		defaultTimeout: 30 * time.Minute,
		maxTimeout:     2 * time.Hour,
	}

	// Clean up any stale locks on startup
	if err := lm.cleanupStaleLocks(); err != nil {
		return nil, fmt.Errorf("failed to cleanup stale locks: %v", err)
	}

	return lm, nil
}

// AcquireLock attempts to acquire a lock on a repository.
func (lm *LockManager) AcquireLock(ctx context.Context, runID, repository string, lockType LockType) error {
	return lm.AcquireLockWithTimeout(ctx, runID, repository, lockType, lm.defaultTimeout)
}

// AcquireLockWithTimeout attempts to acquire a lock with a specific timeout.
func (lm *LockManager) AcquireLockWithTimeout(ctx context.Context, runID, repository string, lockType LockType, timeout time.Duration) error {
	if timeout > lm.maxTimeout {
		timeout = lm.maxTimeout
	}

	lockKey := lm.getLockKey(repository, lockType)
	lockFile := filepath.Join(lm.lockDir, lockKey+".lock")

	// Check for existing conflicting locks
	if err := lm.checkConflictingLocks(repository, lockType); err != nil {
		return err
	}

	// Create lock info
	lockInfo := &LockInfo{
		RunID:      runID,
		Repository: repository,
		Type:       lockType,
		AcquiredAt: time.Now(),
		ExpiresAt:  time.Now().Add(timeout),
		ProcessID:  os.Getpid(),
	}

	// Try to acquire lock with exponential backoff
	const maxRetries = 10
	const baseDelay = 100 * time.Millisecond

	for retry := 0; retry < maxRetries; retry++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := lm.tryAcquireLock(lockFile, lockInfo); err == nil {
			// Successfully acquired lock
			lm.mu.Lock()
			lm.locks[lockKey] = lockInfo
			lm.mu.Unlock()

			return nil
		}

		// Lock acquisition failed, wait with exponential backoff
		delay := baseDelay * time.Duration(1<<uint(retry))
		if delay > 5*time.Second {
			delay = 5 * time.Second
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue with next retry
		}
	}

	return fmt.Errorf("failed to acquire %s lock on repository %s after %d retries", lockType, repository, maxRetries)
}

// ReleaseLock releases a previously acquired lock.
func (lm *LockManager) ReleaseLock(runID, repository string, lockType LockType) error {
	lockKey := lm.getLockKey(repository, lockType)
	lockFile := filepath.Join(lm.lockDir, lockKey+".lock")

	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Verify we own this lock
	lockInfo, exists := lm.locks[lockKey]
	if !exists {
		return fmt.Errorf("no %s lock held on repository %s", lockType, repository)
	}

	if lockInfo.RunID != runID {
		return fmt.Errorf("lock on repository %s is held by run %s, not %s", repository, lockInfo.RunID, runID)
	}

	// Remove lock file
	if err := os.Remove(lockFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove lock file: %v", err)
	}

	// Remove from in-memory tracking
	delete(lm.locks, lockKey)

	return nil
}

// ReleaseAllLocks releases all locks held by a specific run ID.
func (lm *LockManager) ReleaseAllLocks(runID string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	var errs []error

	for lockKey, lockInfo := range lm.locks {
		if lockInfo.RunID == runID {
			lockFile := filepath.Join(lm.lockDir, lockKey+".lock")

			if err := os.Remove(lockFile); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("failed to remove lock file %s: %v", lockFile, err))
			}

			delete(lm.locks, lockKey)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors releasing locks: %v", errs)
	}

	return nil
}

// GetLockInfo returns information about locks held on a repository.
func (lm *LockManager) GetLockInfo(repository string) ([]*LockInfo, error) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var locks []*LockInfo

	// Check for read locks
	readKey := lm.getLockKey(repository, LockTypeRead)
	if lockInfo, exists := lm.locks[readKey]; exists {
		locks = append(locks, lockInfo)
	}

	// Check for write locks
	writeKey := lm.getLockKey(repository, LockTypeWrite)
	if lockInfo, exists := lm.locks[writeKey]; exists {
		locks = append(locks, lockInfo)
	}

	return locks, nil
}

// IsLocked returns true if the repository has any active locks
func (lm *LockManager) IsLocked(repository string) bool {
	locks, _ := lm.GetLockInfo(repository)
	return len(locks) > 0
}

// DetectDeadlocks performs deadlock detection across all active locks
func (lm *LockManager) DetectDeadlocks() ([]string, error) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	// For now, implement a simple timeout-based deadlock detection
	// In a more sophisticated implementation, this would build a wait-for graph
	// and detect cycles

	var deadlocks []string
	now := time.Now()

	for lockKey, lockInfo := range lm.locks {
		if now.After(lockInfo.ExpiresAt) {
			deadlocks = append(deadlocks, fmt.Sprintf("expired lock: %s held by run %s", lockKey, lockInfo.RunID))
		}
	}

	return deadlocks, nil
}

// Close cleans up the lock manager and releases all held locks
func (lm *LockManager) Close() error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Release all locks
	for lockKey := range lm.locks {
		lockFile := filepath.Join(lm.lockDir, lockKey+".lock")
		os.Remove(lockFile) // Ignore errors during cleanup
	}

	lm.locks = make(map[string]*LockInfo)

	return nil
}

// tryAcquireLock attempts to atomically acquire a lock by creating a lock file
func (lm *LockManager) tryAcquireLock(lockFile string, lockInfo *LockInfo) error {
	// Check if lock file already exists
	if _, err := os.Stat(lockFile); err == nil {
		// Lock file exists, check if it's stale
		if err := lm.checkStaleLock(lockFile); err != nil {
			return fmt.Errorf("lock file exists and is not stale: %s", lockFile)
		}
		// Stale lock was removed, try again
	}

	// Create lock file atomically
	data, err := json.Marshal(lockInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal lock info: %v", err)
	}

	// Use O_CREATE|O_EXCL for atomic creation
	file, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create lock file: %v", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		os.Remove(lockFile) // Clean up on failure
		return fmt.Errorf("failed to write lock file: %v", err)
	}

	return nil
}

// checkConflictingLocks checks for conflicting locks before acquiring a new one
func (lm *LockManager) checkConflictingLocks(repository string, lockType LockType) error {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	if lockType == LockTypeWrite {
		// Write locks conflict with all other locks
		readKey := lm.getLockKey(repository, LockTypeRead)
		if _, exists := lm.locks[readKey]; exists {
			return fmt.Errorf("cannot acquire write lock: read lock exists on repository %s", repository)
		}

		writeKey := lm.getLockKey(repository, LockTypeWrite)
		if _, exists := lm.locks[writeKey]; exists {
			return fmt.Errorf("cannot acquire write lock: write lock exists on repository %s", repository)
		}
	} else {
		// Read locks conflict only with write locks
		writeKey := lm.getLockKey(repository, LockTypeWrite)
		if _, exists := lm.locks[writeKey]; exists {
			return fmt.Errorf("cannot acquire read lock: write lock exists on repository %s", repository)
		}
	}

	return nil
}

// checkStaleLock checks if a lock file represents a stale lock and removes it if so
func (lm *LockManager) checkStaleLock(lockFile string) error {
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return fmt.Errorf("failed to read lock file: %v", err)
	}

	var lockInfo LockInfo
	if err := json.Unmarshal(data, &lockInfo); err != nil {
		// Invalid lock file, consider it stale
		os.Remove(lockFile)
		return nil
	}

	// Check if lock has expired
	if time.Now().After(lockInfo.ExpiresAt) {
		os.Remove(lockFile)
		return nil
	}

	// Check if the process that created the lock is still running
	if !lm.isProcessAlive(lockInfo.ProcessID) {
		// Process is dead, remove stale lock
		os.Remove(lockFile)
		return nil
	}

	return fmt.Errorf("lock is still valid")
}

// cleanupStaleLocks removes stale lock files on startup
func (lm *LockManager) cleanupStaleLocks() error {
	entries, err := os.ReadDir(lm.lockDir)
	if err != nil {
		return fmt.Errorf("failed to read lock directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".lock" {
			lockFile := filepath.Join(lm.lockDir, entry.Name())
			lm.checkStaleLock(lockFile) // This will remove stale locks
		}
	}

	return nil
}

// getLockKey generates a unique key for a repository and lock type combination
func (lm *LockManager) getLockKey(repository string, lockType LockType) string {
	// Create a unique key that prevents conflicts between repositories
	// with the same base name but different paths/organizations

	// Normalize the repository path by cleaning it
	normalizedRepo := filepath.Clean(repository)

	// Create a hash of the full repository path to ensure uniqueness
	// while keeping the key filesystem-safe
	hasher := sha256.New()
	hasher.Write([]byte(normalizedRepo))
	hash := hex.EncodeToString(hasher.Sum(nil))[:16] // Use first 16 chars for brevity

	// Create a human-readable base name for easier debugging
	baseName := filepath.Base(normalizedRepo)
	// Sanitize the base name to be filesystem-safe
	safeName := strings.ReplaceAll(baseName, "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")
	safeName = strings.ReplaceAll(safeName, ":", "_")

	// Combine safe name, hash, and lock type for a unique key
	return fmt.Sprintf("%s_%s_%s", safeName, hash, lockType)
}

// isProcessAlive checks if a process with the given PID is still running
func (lm *LockManager) isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	// Platform-specific process checking
	switch runtime.GOOS {
	case "windows":
		// On Windows, use tasklist to check if process exists
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		// If process exists, tasklist will return a line with the process info
		return len(strings.TrimSpace(string(output))) > 0 && !strings.Contains(string(output), "INFO: No tasks are running")

	default:
		// On Unix-like systems (Linux, macOS), try to send signal 0 to check process existence
		// Signal 0 doesn't actually send a signal but checks if we can send to the process
		process, err := os.FindProcess(pid)
		if err != nil {
			return false
		}

		// Try to send a null signal (signal 0) to check if process exists
		err = process.Signal(os.Signal(nil))
		if err != nil {
			// If we get permission denied, the process exists but we can't signal it
			// If we get no such process, the process doesn't exist
			return !strings.Contains(err.Error(), "no such process")
		}

		return true
	}
}
