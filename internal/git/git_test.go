package git_test

import (
	"github.com/dangazineu/tako/internal/git"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestCheckout(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a git repository
	repoPath := filepath.Join(tmpDir, "repo")
	cmd := exec.Command("git", "init", repoPath)
	err := cmd.Run()
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	// Configure git user
	cmd = exec.Command("git", "-C", repoPath, "config", "user.email", "you@example.com")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("failed to set git user.email: %v", err)
	}
	cmd = exec.Command("git", "-C", repoPath, "config", "user.name", "Your Name")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("failed to set git user.name: %v", err)
	}

	// Create an initial commit on main
	cmd = exec.Command("git", "-C", repoPath, "checkout", "-b", "main")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("failed to create main branch: %v", err)
	}
	cmd = exec.Command("git", "-C", repoPath, "commit", "--allow-empty", "-m", "initial commit")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Create a new branch
	cmd = exec.Command("git", "-C", repoPath, "checkout", "-b", "test-branch")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}

	// Checkout the main branch
	err = git.Checkout(repoPath, "main")
	if err != nil {
		t.Fatalf("failed to checkout main branch: %v", err)
	}

	// Verify the current branch
	cmd = exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get current branch: %v", err)
	}
	if strings.TrimSpace(string(output)) != "main" {
		t.Errorf("expected to be on branch 'main', but on '%s'", string(output))
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
