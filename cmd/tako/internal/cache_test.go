package internal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCacheCleanCmd(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Execute the cache clean command
	b := bytes.NewBufferString("")
	cmd := NewRootCmd()
	cmd.SetOut(b)
	cmd.SetArgs([]string{"cache", "clean", "--cache-dir", tmpDir, "--confirm"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute cache clean command: %v", err)
	}

	// Check the output
	expected := "Cache cleaned successfully."
	if !strings.Contains(b.String(), expected) {
		t.Errorf("expected output to contain %q, got %q", expected, b.String())
	}
}

func TestCleanOld(t *testing.T) {
	tmpDir := t.TempDir()
	reposDir := filepath.Join(tmpDir, "repos")
	if err := os.MkdirAll(reposDir, 0755); err != nil {
		t.Fatalf("failed to create repos dir: %v", err)
	}

	// Create a new file and an old file
	newFile := filepath.Join(reposDir, "new")
	oldFile := filepath.Join(reposDir, "old")
	if err := os.Mkdir(newFile, 0755); err != nil {
		t.Fatalf("failed to create new file: %v", err)
	}
	if err := os.Mkdir(oldFile, 0755); err != nil {
		t.Fatalf("failed to create old file: %v", err)
	}
	twoDaysAgo := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, twoDaysAgo, twoDaysAgo); err != nil {
		t.Fatalf("failed to change old file mod time: %v", err)
	}

	// Clean old files
	if err := CleanOld(tmpDir, 24*time.Hour); err != nil {
		t.Fatalf("failed to clean old files: %v", err)
	}

	// Check that the old file is gone and the new one is not
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("expected old file to be gone, but it is not")
	}
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Errorf("expected new file to be there, but it is not")
	}
}