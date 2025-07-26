package engine

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateRunID(t *testing.T) {
	// Generate a run ID
	runID := GenerateRunID()

	// Check format: exec-YYYYMMDD-HHMMSS-<hash>
	if !strings.HasPrefix(runID, "exec-") {
		t.Errorf("Run ID should start with 'exec-', got: %s", runID)
	}

	// Check format: exec- + 8 + - + 6 + - + 8 = 29 characters total
	// exec-20240726-143022-a7b3c1d2
	if len(runID) != 29 {
		t.Errorf("Run ID should be 29 characters long, got %d: %s", len(runID), runID)
	}

	// Verify it can be parsed
	timestamp, hash, err := ParseRunID(runID)
	if err != nil {
		t.Errorf("Generated run ID should be parseable: %v", err)
	}

	// Verify timestamp is recent (within last minute)
	now := time.Now().UTC()
	timestampUTC := timestamp.UTC()
	if timestampUTC.After(now) || timestampUTC.Before(now.Add(-time.Minute)) {
		t.Errorf("Timestamp should be recent, got: %v (now: %v)", timestampUTC, now)
	}

	// Verify hash is 8 characters
	if len(hash) != 8 {
		t.Errorf("Hash should be 8 characters, got %d: %s", len(hash), hash)
	}
}

func TestGenerateRunID_Uniqueness(t *testing.T) {
	// Generate multiple run IDs and ensure they're unique
	runIDs := make(map[string]bool)
	for i := 0; i < 100; i++ {
		runID := GenerateRunID()
		if runIDs[runID] {
			t.Errorf("Generated duplicate run ID: %s", runID)
		}
		runIDs[runID] = true
	}
}

func TestParseRunID(t *testing.T) {
	tests := []struct {
		name        string
		runID       string
		expectError bool
	}{
		{
			name:        "valid run ID",
			runID:       "exec-20240726-143022-a7b3c1d2",
			expectError: false,
		},
		{
			name:        "invalid prefix",
			runID:       "run-20240726-143022-a7b3c1d2",
			expectError: true,
		},
		{
			name:        "too short",
			runID:       "exec-20240726-143022",
			expectError: true,
		},
		{
			name:        "invalid timestamp",
			runID:       "exec-20241301-143022-a7b3c1d2",
			expectError: true,
		},
		{
			name:        "invalid hash length",
			runID:       "exec-20240726-143022-a7b3",
			expectError: true,
		},
		{
			name:        "missing dash before hash",
			runID:       "exec-20240726-143022a7b3c1d2",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp, hash, err := ParseRunID(tt.runID)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for run ID: %s", tt.runID)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for valid run ID %s: %v", tt.runID, err)
				}

				// For valid cases, verify the parsed components
				expectedTime, _ := time.Parse("20060102-150405", "20240726-143022")
				if !timestamp.Equal(expectedTime) {
					t.Errorf("Expected timestamp %v, got %v", expectedTime, timestamp)
				}

				if hash != "a7b3c1d2" {
					t.Errorf("Expected hash 'a7b3c1d2', got '%s'", hash)
				}
			}
		})
	}
}

func TestIsValidRunID(t *testing.T) {
	tests := []struct {
		runID string
		valid bool
	}{
		{"exec-20240726-143022-a7b3c1d2", true},
		{"exec-20241301-143022-a7b3c1d2", false},
		{"run-20240726-143022-a7b3c1d2", false},
		{"exec-20240726-143022", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.runID, func(t *testing.T) {
			result := IsValidRunID(tt.runID)
			if result != tt.valid {
				t.Errorf("IsValidRunID(%s) = %v, expected %v", tt.runID, result, tt.valid)
			}
		})
	}
}

func BenchmarkGenerateRunID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateRunID()
	}
}

func BenchmarkParseRunID(b *testing.B) {
	runID := "exec-20240726-143022-a7b3c1d2"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ParseRunID(runID)
	}
}
