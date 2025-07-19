package graph_test

import (
	"bytes"
	"github.com/dangazineu/tako/internal/graph"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPrintGraph(t *testing.T) {
	root := &graph.Node{
		Name: "root",
		Children: []*graph.Node{
			{
				Name: "child1",
				Children: []*graph.Node{
					{Name: "grandchild"},
				},
			},
			{Name: "child2"},
		},
	}

	var buf bytes.Buffer
	graph.PrintGraph(&buf, root)

	expected := `root
├── child1
│   └── grandchild
└── child2
`
	if expected != buf.String() {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestBuildGraph(t *testing.T) {
	t.Run("local", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create mock repositories
		repoA := filepath.Join(tmpDir, "repo-a")
		repoB := filepath.Join(tmpDir, "repo-b")
		repoC := filepath.Join(tmpDir, "repo-c")

		if err := os.Mkdir(repoA, 0755); err != nil {
			t.Fatalf("failed to create repoA: %v", err)
		}
		if err := os.Mkdir(repoB, 0755); err != nil {
			t.Fatalf("failed to create repoB: %v", err)
		}
		if err := os.Mkdir(repoC, 0755); err != nil {
			t.Fatalf("failed to create repoC: %v", err)
		}

		// Create mock tako.yml files
		takoA := `
version: 0.1.0
metadata:
  name: repo-a
dependents:
  - repo: ../repo-b:main
`
		takoB := `
version: 0.1.0
metadata:
  name: repo-b
dependents:
  - repo: ../repo-c:main
`
		takoC := `
version: 0.1.0
metadata:
  name: repo-c
dependents: []
`
		err := os.WriteFile(filepath.Join(repoA, "tako.yml"), []byte(takoA), 0644)
		if err != nil {
			t.Fatalf("failed to write tako.yml: %v", err)
		}
		err = os.WriteFile(filepath.Join(repoB, "tako.yml"), []byte(takoB), 0644)
		if err != nil {
			t.Fatalf("failed to write tako.yml: %v", err)
		}
		err = os.WriteFile(filepath.Join(repoC, "tako.yml"), []byte(takoC), 0644)
		if err != nil {
			t.Fatalf("failed to write tako.yml: %v", err)
		}

		// Build the graph in local-only mode
		cacheDir := t.TempDir()
		root, err := graph.BuildGraph(repoA, cacheDir, true)
		if err != nil {
			t.Fatalf("failed to build graph: %v", err)
		}

		// Assert the graph is correct
		if root.Name != "repo-a" {
			t.Errorf("expected root name to be 'repo-a', got %q", root.Name)
		}
		if len(root.Children) != 1 {
			t.Fatalf("expected root to have 1 child, got %d", len(root.Children))
		}
		childB := root.Children[0]
		if childB.Name != "repo-b" {
			t.Errorf("expected child name to be 'repo-b', got %q", childB.Name)
		}
		if len(childB.Children) != 1 {
			t.Fatalf("expected childB to have 1 child, got %d", len(childB.Children))
		}
		childC := childB.Children[0]
		if childC.Name != "repo-c" {
			t.Errorf("expected child name to be 'repo-c', got %q", childC.Name)
		}
	})

	t.Run("remote", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create mock repositories
		repoA := filepath.Join(tmpDir, "repo-a")
		repoB := filepath.Join(tmpDir, "repo-b")

		if err := os.Mkdir(repoA, 0755); err != nil {
			t.Fatalf("failed to create repoA: %v", err)
		}
		if err := os.Mkdir(repoB, 0755); err != nil {
			t.Fatalf("failed to create repoB: %v", err)
		}

		// Init git repo in repoB
		cmd := exec.Command("git", "init")
		cmd.Dir = repoB
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to init git repo: %v", err)
		}

		// Create mock tako.yml files
		takoA := `
version: 0.1.0
metadata:
  name: repo-a
dependents:
  - repo: file://` + repoB + `:main
`
		takoB := `
version: 0.1.0
metadata:
  name: repo-b
dependents: []
`
		err := os.WriteFile(filepath.Join(repoA, "tako.yml"), []byte(takoA), 0644)
		if err != nil {
			t.Fatalf("failed to write tako.yml: %v", err)
		}
		err = os.WriteFile(filepath.Join(repoB, "tako.yml"), []byte(takoB), 0644)
		if err != nil {
			t.Fatalf("failed to write tako.yml: %v", err)
		}

		// Build the graph in remote mode
		cacheDir := t.TempDir()
		root, err := graph.BuildGraph(repoA, cacheDir, false)
		if err != nil {
			t.Fatalf("failed to build graph: %v", err)
		}

		// Assert the graph is correct
		if root.Name != "repo-a" {
			t.Errorf("expected root name to be 'repo-a', got %q", root.Name)
		}
		if len(root.Children) != 1 {
			t.Fatalf("expected root to have 1 child, got %d", len(root.Children))
		}
		childB := root.Children[0]
		if childB.Name != "repo-b" {
			t.Errorf("expected child name to be 'repo-b', got %q", childB.Name)
		}
	})
	t.Run("remote-not-cached", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create mock repositories
		repoA := filepath.Join(tmpDir, "repo-a")
		repoB := filepath.Join(tmpDir, "repo-b")

		if err := os.Mkdir(repoA, 0755); err != nil {
			t.Fatalf("failed to create repoA: %v", err)
		}
		if err := os.Mkdir(repoB, 0755); err != nil {
			t.Fatalf("failed to create repoB: %v", err)
		}

		// Init git repo in repoB
		cmd := exec.Command("git", "init")
		cmd.Dir = repoB
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to init git repo: %v", err)
		}
		cmd = exec.Command("git", "config", "user.email", "you@example.com")
		cmd.Dir = repoB
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to set git user.email: %v", err)
		}
		cmd = exec.Command("git", "config", "user.name", "Your Name")
		cmd.Dir = repoB
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to set git user.name: %v", err)
		}
		cmd = exec.Command("git", "commit", "--allow-empty", "-m", "initial commit")
		cmd.Dir = repoB
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to commit: %v", err)
		}

		// Create mock tako.yml files
		takoA := `
version: 0.1.0
metadata:
  name: repo-a
dependents:
  - repo: file://` + repoB + `:main
`
		takoB := `
version: 0.1.0
metadata:
  name: repo-b
dependents: []
`
		err := os.WriteFile(filepath.Join(repoA, "tako.yml"), []byte(takoA), 0644)
		if err != nil {
			t.Fatalf("failed to write tako.yml: %v", err)
		}
		err = os.WriteFile(filepath.Join(repoB, "tako.yml"), []byte(takoB), 0644)
		if err != nil {
			t.Fatalf("failed to write tako.yml: %v", err)
		}

		// Build the graph in remote mode
		cacheDir := t.TempDir()
		root, err := graph.BuildGraph(repoA, cacheDir, false)
		if err != nil {
			t.Fatalf("failed to build graph: %v", err)
		}

		// Assert the graph is correct
		if root.Name != "repo-a" {
			t.Errorf("expected root name to be 'repo-a', got %q", root.Name)
		}
		if len(root.Children) != 1 {
			t.Fatalf("expected root to have 1 child, got %d", len(root.Children))
		}
		childB := root.Children[0]
		if childB.Name != "repo-b" {
			t.Errorf("expected child name to be 'repo-b', got %q", childB.Name)
		}
	})

	t.Run("circular-dependency", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create mock repositories
		repoA := filepath.Join(tmpDir, "repo-a")
		repoB := filepath.Join(tmpDir, "repo-b")

		if err := os.Mkdir(repoA, 0755); err != nil {
			t.Fatalf("failed to create repoA: %v", err)
		}
		if err := os.Mkdir(repoB, 0755); err != nil {
			t.Fatalf("failed to create repoB: %v", err)
		}

		// Create mock tako.yml files with circular dependency
		takoA := `
version: 0.1.0
metadata:
  name: repo-a
dependents:
  - repo: ../repo-b:main
`
		takoB := `
version: 0.1.0
metadata:
  name: repo-b
dependents:
  - repo: ../repo-a:main
`
		err := os.WriteFile(filepath.Join(repoA, "tako.yml"), []byte(takoA), 0644)
		if err != nil {
			t.Fatalf("failed to write tako.yml: %v", err)
		}
		err = os.WriteFile(filepath.Join(repoB, "tako.yml"), []byte(takoB), 0644)
		if err != nil {
			t.Fatalf("failed to write tako.yml: %v", err)
		}

		// Build the graph and expect a circular dependency error
		cacheDir := t.TempDir()
		_, err = graph.BuildGraph(repoA, cacheDir, true)
		if err == nil {
			t.Fatal("expected a circular dependency error, but got nil")
		}

		cdErr, ok := err.(*graph.CircularDependencyError)
		if !ok {
			t.Fatalf("expected error to be of type *graph.CircularDependencyError, but got %T", err)
		}

		expectedError := "circular dependency detected: repo-a -> repo-b -> repo-a"
		if cdErr.Error() != expectedError {
			t.Errorf("expected error message %q, but got %q", expectedError, cdErr.Error())
		}
	})

	t.Run("get-repo-path-error", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoA := filepath.Join(tmpDir, "repo-a")
		if err := os.Mkdir(repoA, 0755); err != nil {
			t.Fatalf("failed to create repoA: %v", err)
		}

		// Create a mock tako.yml with an invalid repo path
		takoA := `
version: 0.1.0
metadata:
  name: repo-a
dependents:
  - repo: invalid-repo-path
`
		err := os.WriteFile(filepath.Join(repoA, "tako.yml"), []byte(takoA), 0644)
		if err != nil {
			t.Fatalf("failed to write tako.yml: %v", err)
		}

		// Build the graph and expect an error
		cacheDir := t.TempDir()
		_, err = graph.BuildGraph(repoA, cacheDir, true)
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
	})
}
