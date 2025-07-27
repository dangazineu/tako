package engine

import (
	"crypto/md5"
	"fmt"
	"math/rand"
	"time"
)

// GenerateRunID generates a timestamp-based run ID for human readability.
// Format: exec-YYYYMMDD-HHMMSS-<hash>
// Example: exec-20240726-143022-a7b3c1d2.
func GenerateRunID() string {
	now := time.Now().UTC()

	// Create timestamp portion
	timestamp := now.Format("20060102-150405")

	// Generate collision-resistant hash using timestamp and random seed
	// This ensures uniqueness even if multiple runs start at the same second
	source := fmt.Sprintf("%d-%d", now.UnixNano(), rand.Int63())
	hash := md5.Sum([]byte(source))
	shortHash := fmt.Sprintf("%x", hash)[:8]

	return fmt.Sprintf("exec-%s-%s", timestamp, shortHash)
}

// ParseRunID extracts components from a run ID
// Returns timestamp, hash, and error if parsing fails.
func ParseRunID(runID string) (time.Time, string, error) {
	if len(runID) < 29 || runID[:5] != "exec-" {
		return time.Time{}, "", fmt.Errorf("invalid run ID format: %s", runID)
	}

	// Extract timestamp portion (positions 5-20: YYYYMMDD-HHMMSS)
	timestampStr := runID[5:20]
	timestamp, err := time.Parse("20060102-150405", timestampStr)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid timestamp in run ID %s: %v", runID, err)
	}

	// Extract hash portion (after the second dash)
	if len(runID) < 21 || runID[20] != '-' {
		return time.Time{}, "", fmt.Errorf("invalid run ID format: missing hash portion in %s", runID)
	}

	hash := runID[21:]
	if len(hash) != 8 {
		return time.Time{}, "", fmt.Errorf("invalid hash length in run ID %s: expected 8 characters, got %d", runID, len(hash))
	}

	return timestamp, hash, nil
}

// IsValidRunID checks if a string follows the expected run ID format.
func IsValidRunID(runID string) bool {
	_, _, err := ParseRunID(runID)
	return err == nil
}
