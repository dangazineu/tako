package graph

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAllNodes(t *testing.T) {
	// Create a simple graph
	// A -> B -> C
	//   -> D
	nodeD := &Node{Name: "D"}
	nodeC := &Node{Name: "C"}
	nodeB := &Node{Name: "B", Children: []*Node{nodeC}}
	nodeA := &Node{Name: "A", Children: []*Node{nodeB, nodeD}}

	nodes := nodeA.AllNodes()
	if len(nodes) != 4 {
		t.Errorf("Expected 4 nodes in the graph, but got %d", len(nodes))
	}

	names := make(map[string]bool)
	for _, n := range nodes {
		names[n.Name] = true
	}

	for _, name := range []string{"A", "B", "C", "D"} {
		if !names[name] {
			t.Errorf("Expected to find node %s", name)
		}
	}
}

func TestTopologicalSort(t *testing.T) {
	// Create a simple graph
	// A -> B -> C
	//   -> D
	nodeD := &Node{Name: "D"}
	nodeC := &Node{Name: "C"}
	nodeB := &Node{Name: "B", Children: []*Node{nodeC}}
	nodeA := &Node{Name: "A", Children: []*Node{nodeB, nodeD}}

	sorted, err := nodeA.TopologicalSort()
	if err != nil {
		t.Fatalf("Expected no error, but got %v", err)
	}
	if len(sorted) != 4 {
		t.Fatalf("Expected 4 sorted nodes, but got %d", len(sorted))
	}

	// Check the order
	names := make([]string, len(sorted))
	for i, n := range sorted {
		names[i] = n.Name
	}
	expected := []string{"A", "B", "C", "D"}
	if !reflect.DeepEqual(names, expected) {
		t.Errorf("Expected sorted order %v, but got %v", expected, names)
	}
}

func TestTopologicalSort_CircularDependency(t *testing.T) {
	// Create a graph with a circular dependency
	// A -> B -> C -> A
	nodeA := &Node{Name: "A"}
	nodeC := &Node{Name: "C", Children: []*Node{nodeA}}
	nodeB := &Node{Name: "B", Children: []*Node{nodeC}}
	nodeA.Children = []*Node{nodeB}

	_, err := nodeA.TopologicalSort()
	if err == nil {
		t.Fatal("Expected an error, but got nil")
	}
	if _, ok := err.(*CircularDependencyError); !ok {
		t.Errorf("Expected error of type *CircularDependencyError, but got %T", err)
	}
}

func TestBuildGraph_CircularDependency(t *testing.T) {
	tmpDir := t.TempDir()

	// Create repo-a
	repoA := filepath.Join(tmpDir, "repo-a")
	if err := os.Mkdir(repoA, 0755); err != nil {
		t.Fatal(err)
	}
	takoA := `
version: 0.1.0
metadata:
  name: repo-a
dependents:
  - repo: local/repo-b:main
`
	if err := os.WriteFile(filepath.Join(repoA, "tako.yml"), []byte(takoA), 0644); err != nil {
		t.Fatal(err)
	}

	// Create repo-b
	repoB := filepath.Join(tmpDir, "repo-b")
	if err := os.Mkdir(repoB, 0755); err != nil {
		t.Fatal(err)
	}
	takoB := `
version: 0.1.0
metadata:
  name: repo-b
dependents:
  - repo: local/repo-a:main
`
	if err := os.WriteFile(filepath.Join(repoB, "tako.yml"), []byte(takoB), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.Mkdir(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create symlinks in the cache to simulate local repos
	if err := os.MkdirAll(filepath.Join(cacheDir, "repos", "local"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repoA, filepath.Join(cacheDir, "repos", "local", "repo-a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repoB, filepath.Join(cacheDir, "repos", "local", "repo-b")); err != nil {
		t.Fatal(err)
	}

	_, err := BuildGraph(repoA, cacheDir, true)
	if err == nil {
		t.Fatal("Expected an error, but got nil")
	}
	if _, ok := err.(*CircularDependencyError); !ok {
		t.Errorf("Expected error of type *CircularDependencyError, but got %T", err)
	}
	expectedError := "circular dependency detected: repo-a -> repo-b -> repo-a"
	if err.Error() != expectedError {
		t.Errorf("Expected error message '%s', but got '%s'", expectedError, err.Error())
	}
}

func TestPrintDot(t *testing.T) {
	// A -> B -> C
	//   -> D
	nodeD := &Node{Name: "D"}
	nodeC := &Node{Name: "C"}
	nodeB := &Node{Name: "B", Children: []*Node{nodeC}}
	nodeA := &Node{Name: "A", Children: []*Node{nodeB, nodeD}}

	var buf bytes.Buffer
	PrintDot(&buf, nodeA)

	expected := `digraph {
  "A" [label="A"];
  "A" -> "B";
  "B" [label="B"];
  "B" -> "C";
  "C" [label="C"];
  "A" -> "D";
  "D" [label="D"];
}
`
	if buf.String() != expected {
		t.Errorf("Expected dot output '%s', but got '%s'", expected, buf.String())
	}
}
