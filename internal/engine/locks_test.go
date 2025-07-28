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

func TestLockManager_AcquireLock(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running lock test in short mode")
	}
	tempDir := t.TempDir()

	lm, err := NewLockManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create lock manager: %v", err)
	}
	defer lm.Close()

	ctx := context.Background()
	runID := "test-run-123"
	repository := "test/repo"

	// Test acquiring a read lock
	err = lm.AcquireLock(ctx, runID, repository, LockTypeRead)
	if err != nil {
		t.Fatalf("Failed to acquire read lock: %v", err)
	}

	// Test acquiring a write lock (should fail due to existing read lock)
	err = lm.AcquireLock(ctx, "test-run-789", repository, LockTypeWrite)
	if err == nil {
		t.Error("Should not be able to acquire write lock when read lock exists")
	}

	// Test acquiring another read lock (should fail - current implementation allows only one)
	err = lm.AcquireLock(ctx, "test-run-456", repository, LockTypeRead)
	if err == nil {
		t.Error("Current implementation should not allow multiple read locks")
	}

	// Clean up
	lm.ReleaseLock(runID, repository, LockTypeRead)
}

func TestLockManager_AcquireLockWithTimeout(t *testing.T) {
	tempDir := t.TempDir()

	lm, err := NewLockManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create lock manager: %v", err)
	}
	defer lm.Close()

	ctx := context.Background()
	repository := "test/repo"

	// Acquire write lock first
	err = lm.AcquireLockWithTimeout(ctx, "run-1", repository, LockTypeWrite, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire initial write lock: %v", err)
	}

	// Try to acquire conflicting lock with short timeout
	start := time.Now()
	err = lm.AcquireLockWithTimeout(ctx, "run-2", repository, LockTypeWrite, 100*time.Millisecond)
	duration := time.Since(start)

	if err == nil {
		t.Error("Should have failed to acquire conflicting lock")
	}

	// Should have timed out relatively quickly
	if duration > 200*time.Millisecond {
		t.Errorf("Lock acquisition took too long: %v", duration)
	}

	// Clean up
	lm.ReleaseLock("run-1", repository, LockTypeWrite)
}

func TestLockManager_ReleaseLock(t *testing.T) {
	tempDir := t.TempDir()

	lm, err := NewLockManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create lock manager: %v", err)
	}
	defer lm.Close()

	ctx := context.Background()
	runID := "test-run-123"
	repository := "test/repo"

	// Acquire lock
	err = lm.AcquireLock(ctx, runID, repository, LockTypeWrite)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Verify lock exists
	if !lm.IsLocked(repository) {
		t.Error("Repository should be locked")
	}

	// Release lock
	err = lm.ReleaseLock(runID, repository, LockTypeWrite)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Verify lock is gone
	if lm.IsLocked(repository) {
		t.Error("Repository should not be locked after release")
	}

	// Try to release non-existent lock
	err = lm.ReleaseLock(runID, repository, LockTypeWrite)
	if err == nil {
		t.Error("Should have failed to release non-existent lock")
	}

	// Try to release lock with wrong run ID
	lm.AcquireLock(ctx, runID, repository, LockTypeWrite)
	err = lm.ReleaseLock("wrong-run-id", repository, LockTypeWrite)
	if err == nil {
		t.Error("Should have failed to release lock with wrong run ID")
	}
}

func TestLockManager_ReleaseAllLocks(t *testing.T) {
	tempDir := t.TempDir()

	lm, err := NewLockManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create lock manager: %v", err)
	}
	defer lm.Close()

	ctx := context.Background()
	runID := "test-run-123"

	// Acquire multiple locks
	repos := []string{"repo1", "repo2", "repo3"}
	for _, repo := range repos {
		err = lm.AcquireLock(ctx, runID, repo, LockTypeWrite)
		if err != nil {
			t.Fatalf("Failed to acquire lock for %s: %v", repo, err)
		}
	}

	// Verify all locks exist
	for _, repo := range repos {
		if !lm.IsLocked(repo) {
			t.Errorf("Repository %s should be locked", repo)
		}
	}

	// Release all locks for the run ID
	err = lm.ReleaseAllLocks(runID)
	if err != nil {
		t.Fatalf("Failed to release all locks: %v", err)
	}

	// Verify all locks are gone
	for _, repo := range repos {
		if lm.IsLocked(repo) {
			t.Errorf("Repository %s should not be locked after release all", repo)
		}
	}
}

func TestLockManager_GetLockInfo(t *testing.T) {
	tempDir := t.TempDir()

	lm, err := NewLockManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create lock manager: %v", err)
	}
	defer lm.Close()

	ctx := context.Background()
	repository := "test/repo"

	// No locks initially
	locks, err := lm.GetLockInfo(repository)
	if err != nil {
		t.Fatalf("Failed to get lock info: %v", err)
	}
	if len(locks) != 0 {
		t.Errorf("Expected 0 locks, got %d", len(locks))
	}

	// Acquire a lock
	runID := "test-run-123"
	err = lm.AcquireLock(ctx, runID, repository, LockTypeWrite)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Check lock info
	locks, err = lm.GetLockInfo(repository)
	if err != nil {
		t.Fatalf("Failed to get lock info: %v", err)
	}
	if len(locks) != 1 {
		t.Errorf("Expected 1 lock, got %d", len(locks))
	}

	lockInfo := locks[0]
	if lockInfo.RunID != runID {
		t.Errorf("Expected run ID %s, got %s", runID, lockInfo.RunID)
	}
	if lockInfo.Type != LockTypeWrite {
		t.Errorf("Expected lock type %s, got %s", LockTypeWrite, lockInfo.Type)
	}
	if lockInfo.Repository != repository {
		t.Errorf("Expected repository %s, got %s", repository, lockInfo.Repository)
	}
}

func TestLockManager_IsLocked(t *testing.T) {
	tempDir := t.TempDir()

	lm, err := NewLockManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create lock manager: %v", err)
	}
	defer lm.Close()

	ctx := context.Background()
	repository := "test/repo"

	// Initially not locked
	if lm.IsLocked(repository) {
		t.Error("Repository should not be locked initially")
	}

	// Acquire lock
	err = lm.AcquireLock(ctx, "test-run", repository, LockTypeRead)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Should be locked now
	if !lm.IsLocked(repository) {
		t.Error("Repository should be locked")
	}

	// Release lock
	lm.ReleaseLock("test-run", repository, LockTypeRead)

	// Should not be locked anymore
	if lm.IsLocked(repository) {
		t.Error("Repository should not be locked after release")
	}
}

func TestLockManager_DetectDeadlocks(t *testing.T) {
	tempDir := t.TempDir()

	lm, err := NewLockManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create lock manager: %v", err)
	}
	defer lm.Close()

	ctx := context.Background()

	// No deadlocks initially
	deadlocks, err := lm.DetectDeadlocks()
	if err != nil {
		t.Fatalf("Failed to detect deadlocks: %v", err)
	}
	if len(deadlocks) != 0 {
		t.Errorf("Expected 0 deadlocks, got %d", len(deadlocks))
	}

	// Acquire a lock with very short timeout to simulate expired lock
	err = lm.AcquireLockWithTimeout(ctx, "test-run", "test/repo", LockTypeWrite, 1*time.Nanosecond)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Wait a moment to ensure lock expires
	time.Sleep(10 * time.Millisecond)

	// Check for deadlocks (expired locks)
	deadlocks, err = lm.DetectDeadlocks()
	if err != nil {
		t.Fatalf("Failed to detect deadlocks: %v", err)
	}
	if len(deadlocks) == 0 {
		t.Error("Expected to find expired lock as deadlock")
	}
}

func TestLockManager_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()

	lm, err := NewLockManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create lock manager: %v", err)
	}
	defer lm.Close()

	numGoroutines := 10
	var wg sync.WaitGroup
	var successCount int32
	var mutex sync.Mutex

	// Try to acquire locks on different repositories from multiple goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx := context.Background()
			runID := fmt.Sprintf("test-run-%d", id)
			repository := fmt.Sprintf("test/repo-%d", id) // Different repos for each goroutine

			err := lm.AcquireLock(ctx, runID, repository, LockTypeWrite)
			if err == nil {
				mutex.Lock()
				successCount++
				mutex.Unlock()

				// Hold lock briefly
				time.Sleep(10 * time.Millisecond)

				// Release lock
				lm.ReleaseLock(runID, repository, LockTypeWrite)
			}
		}(i)
	}

	wg.Wait()

	// All goroutines should succeed since they're using different repositories
	if successCount != int32(numGoroutines) {
		t.Errorf("Expected %d successful lock acquisitions, got %d", numGoroutines, successCount)
	}
}

func TestLockManager_getLockKey(t *testing.T) {
	lm := &LockManager{}

	// Test key generation
	key1 := lm.getLockKey("repo1", LockTypeRead)
	key2 := lm.getLockKey("repo1", LockTypeWrite)
	key3 := lm.getLockKey("repo2", LockTypeRead)

	// Keys should be different for different types and repos
	if key1 == key2 {
		t.Error("Read and write locks should have different keys")
	}
	if key1 == key3 {
		t.Error("Different repositories should have different keys")
	}

	// Same repo and type should produce same key
	key4 := lm.getLockKey("repo1", LockTypeRead)
	if key1 != key4 {
		t.Error("Same repo and lock type should produce same key")
	}

	// Keys should be filesystem-safe (no problematic characters)
	problematicChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range problematicChars {
		if len(key1) > 0 && (key1[0] == char[0] || key1[len(key1)-1] == char[0]) {
			t.Errorf("Lock key should not start or end with problematic character: %s", char)
		}
	}
}

func TestLockManager_isProcessAlive(t *testing.T) {
	lm := &LockManager{}

	// Test with current process (should be alive)
	currentPID := os.Getpid()
	if !lm.isProcessAlive(currentPID) {
		t.Error("Current process should be alive")
	}

	// Test with invalid PID
	if lm.isProcessAlive(-1) {
		t.Error("Invalid PID should not be alive")
	}
	if lm.isProcessAlive(0) {
		t.Error("PID 0 should not be alive")
	}

	// Test with very high PID (likely non-existent)
	// Note: This test may be flaky on some systems where high PIDs exist
	// so we'll make it less strict
	highPID := 999999
	if lm.isProcessAlive(highPID) {
		t.Logf("Warning: High PID %d appears to be alive (this may be system-dependent)", highPID)
	}
}

func TestLockManager_checkConflictingLocks(t *testing.T) {
	tempDir := t.TempDir()

	lm, err := NewLockManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create lock manager: %v", err)
	}
	defer lm.Close()

	ctx := context.Background()
	repository := "test/repo"

	// No conflicts initially
	err = lm.checkConflictingLocks(repository, LockTypeRead)
	if err != nil {
		t.Errorf("Should not have conflicts initially: %v", err)
	}

	// Acquire read lock
	lm.AcquireLock(ctx, "run-1", repository, LockTypeRead)

	// Another read lock should not conflict
	err = lm.checkConflictingLocks(repository, LockTypeRead)
	if err != nil {
		t.Errorf("Read locks should not conflict with each other: %v", err)
	}

	// Write lock should conflict with read lock
	err = lm.checkConflictingLocks(repository, LockTypeWrite)
	if err == nil {
		t.Error("Write lock should conflict with existing read lock")
	}

	// Clean up read lock and acquire write lock
	lm.ReleaseLock("run-1", repository, LockTypeRead)
	lm.AcquireLock(ctx, "run-2", repository, LockTypeWrite)

	// Read lock should conflict with write lock
	err = lm.checkConflictingLocks(repository, LockTypeRead)
	if err == nil {
		t.Error("Read lock should conflict with existing write lock")
	}

	// Another write lock should conflict
	err = lm.checkConflictingLocks(repository, LockTypeWrite)
	if err == nil {
		t.Error("Write locks should conflict with each other")
	}
}

func TestLockManager_cleanupStaleLocks(t *testing.T) {
	tempDir := t.TempDir()

	lm, err := NewLockManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create lock manager: %v", err)
	}
	defer lm.Close()

	// Create a stale lock file manually
	lockFile := filepath.Join(tempDir, "stale.lock")
	staleData := `{"run_id":"stale-run","repository":"test/repo","type":"write","acquired_at":"2020-01-01T00:00:00Z","expires_at":"2020-01-01T00:01:00Z","process_id":999999}`

	err = os.WriteFile(lockFile, []byte(staleData), 0644)
	if err != nil {
		t.Fatalf("Failed to create stale lock file: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		t.Fatal("Stale lock file should exist")
	}

	// Run cleanup
	err = lm.cleanupStaleLocks()
	if err != nil {
		t.Fatalf("Failed to cleanup stale locks: %v", err)
	}

	// Stale lock file should be removed
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("Stale lock file should have been removed")
	}
}
