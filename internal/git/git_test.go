package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckout(t *testing.T) {
	// Create a bare repo to act as the origin
	originDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = originDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git init bare: %v", err)
	}

	// Clone the bare repo
	cloneDir := t.TempDir()
	cmd = exec.Command("git", "clone", originDir, cloneDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git clone: %v", err)
	}

	// Create a commit
	if err := os.WriteFile(filepath.Join(cloneDir, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = cloneDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}
	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = cloneDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Push the commit
	cmd = exec.Command("git", "push", "origin", "main")
	cmd.Dir = cloneDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git push: %v", err)
	}

	// Create a new branch
	cmd = exec.Command("git", "branch", "test-branch")
	cmd.Dir = cloneDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git branch: %v", err)
	}

	// Push the new branch
	cmd = exec.Command("git", "push", "origin", "test-branch")
	cmd.Dir = cloneDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git push: %v", err)
	}

	// Create a tag
	cmd = exec.Command("git", "tag", "v1.0.0")
	cmd.Dir = cloneDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git tag: %v", err)
	}

	// Push the tag
	cmd = exec.Command("git", "push", "origin", "v1.0.0")
	cmd.Dir = cloneDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git push: %v", err)
	}

	// Get the commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = cloneDir
	hash, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}
	commitHash := strings.TrimSpace(string(hash))

	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{
			name:    "checkout branch",
			ref:     "test-branch",
			wantErr: false,
		},
		{
			name:    "checkout tag",
			ref:     "v1.0.0",
			wantErr: false,
		},
		{
			name:    "checkout commit hash",
			ref:     commitHash,
			wantErr: false,
		},
		{
			name:    "checkout non-existent ref",
			ref:     "non-existent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Checkout(cloneDir, tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("Checkout() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPull(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git init: %v", err)
	}

	err := Pull(dir)
	if err == nil {
		t.Errorf("Pull() error = %v, wantErr %v", err, true)
	}
}

func TestGetDefaultBranch(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git init: %v", err)
	}

	_, err := GetDefaultBranch(dir)
	if err == nil {
		t.Errorf("GetDefaultBranch() error = %v, wantErr %v", err, true)
	}
}
