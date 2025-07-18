package internal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheCleanCmd(t *testing.T) {
	// Create a temporary cache directory for testing
	tmpDir := t.TempDir()

	// Create a dummy file in the cache
	dummyFile := filepath.Join(tmpDir, "dummy.txt")
	err := os.WriteFile(dummyFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to write dummy file: %v", err)
	}

	// Redirect stdout to a buffer to capture output
	var stdout bytes.Buffer
	rootCmd := NewRootCmd()
	cacheCmd, _, err := rootCmd.Find([]string{"cache"})
	if err != nil {
		t.Fatalf("Failed to find cache command: %v", err)
	}
	cacheCmd.SetOut(&stdout)

	// Execute the command
	rootCmd.SetArgs([]string{"cache", "clean", "--cache-dir", tmpDir, "--confirm"})
	err = rootCmd.Execute()
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	// Assertions
	if !strings.Contains(stdout.String(), "Cleaning cache...") {
		t.Errorf("Expected output to contain 'Cleaning cache...', got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Cache cleaned successfully.") {
		t.Errorf("Expected output to contain 'Cache cleaned successfully.', got %s", stdout.String())
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Errorf("Cache directory should be deleted, but it still exists.")
	}
}
