package git_test

import (
	"github.com/dangazineu/tako/internal/git"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestClone(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a bare git repository
	bareRepoPath := filepath.Join(tmpDir, "bare.git")
	cmd := exec.Command("git", "init", "--bare", bareRepoPath)
	err := cmd.Run()
	if err != nil {
		t.Fatalf("failed to create bare repo: %v", err)
	}

	// Clone the repository
	clonePath := filepath.Join(tmpDir, "clone")
	err = git.Clone(bareRepoPath, clonePath)
	if err != nil {
		t.Fatalf("failed to clone repo: %v", err)
	}

	// Verify the clone
	if _, err := os.Stat(filepath.Join(clonePath, ".git")); os.IsNotExist(err) {
		t.Errorf(".git directory not found in cloned repo")
	}
}
