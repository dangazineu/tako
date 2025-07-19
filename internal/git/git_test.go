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

func TestGetEntrypointPath(t *testing.T) {
	t.Run("with repo flag", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		repoPath := filepath.Join(cacheDir, "repos", "owner", "repo")
		err := os.MkdirAll(repoPath, 0755)
		if err != nil {
			t.Fatalf("failed to create repo path: %v", err)
		}

		path, err := git.GetEntrypointPath("", "owner/repo:main", cacheDir, true)
		if err != nil {
			t.Fatalf("failed to get entrypoint path: %v", err)
		}
		if path != repoPath {
			t.Errorf("expected path %s, got %s", repoPath, path)
		}
	})

	t.Run("without repo flag", func(t *testing.T) {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get working directory: %v", err)
		}
		path, err := git.GetEntrypointPath("", "", "", false)
		if err != nil {
			t.Fatalf("failed to get entrypoint path: %v", err)
		}
		if path != wd {
			t.Errorf("expected path %s, got %s", wd, path)
		}
	})
}