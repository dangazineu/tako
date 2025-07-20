package graph

import (
	"bytes"
	"github.com/stretchr/testify/assert"
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
	// A -> B
	nodeB1 := &Node{Name: "B"}
	nodeA1 := &Node{Name: "A", Children: []*Node{nodeB1}}

	// A -> B, A -> C
	nodeB2 := &Node{Name: "B"}
	nodeC2 := &Node{Name: "C"}
	nodeA2 := &Node{Name: "A", Children: []*Node{nodeB2, nodeC2}}

	// A -> B -> C
	nodeC3 := &Node{Name: "C"}
	nodeB3 := &Node{Name: "B", Children: []*Node{nodeC3}}
	nodeA3 := &Node{Name: "A", Children: []*Node{nodeB3}}

	// Diamond: A -> B, A -> C, B -> D, C -> D
	nodeD4 := &Node{Name: "D"}
	nodeB4 := &Node{Name: "B", Children: []*Node{nodeD4}}
	nodeC4 := &Node{Name: "C", Children: []*Node{nodeD4}}
	nodeA4 := &Node{Name: "A", Children: []*Node{nodeB4, nodeC4}}

	// Circular: A -> B -> A
	nodeA5 := &Node{Name: "A"}
	nodeB5 := &Node{Name: "B", Children: []*Node{nodeA5}}
	nodeA5.Children = []*Node{nodeB5}

	testCases := []struct {
		name          string
		root          *Node
		expectedOrder []string
		expectError   bool
	}{
		{
			name:          "Simple Chain",
			root:          nodeA1,
			expectedOrder: []string{"A", "B"},
			expectError:   false,
		},
		{
			name:          "Single root, multiple children",
			root:          nodeA2,
			expectedOrder: []string{"A", "B", "C"},
			expectError:   false,
		},
		{
			name:          "Longer Chain",
			root:          nodeA3,
			expectedOrder: []string{"A", "B", "C"},
			expectError:   false,
		},
		{
			name:          "Diamond Shape",
			root:          nodeA4,
			expectedOrder: []string{"A", "B", "C", "D"},
			expectError:   false,
		},
		{
			name:        "Circular Dependency",
			root:        nodeA5,
			expectError: true,
		},
		{
			name:          "Single Node",
			root:          &Node{Name: "A"},
			expectedOrder: []string{"A"},
			expectError:   false,
		},
		{
			name:          "Empty Graph",
			root:          &Node{Name: "A", Children: []*Node{}},
			expectedOrder: []string{"A"},
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sorted, err := tc.root.TopologicalSort()

			if tc.expectError {
				if err == nil {
					t.Fatal("Expected an error, but got nil")
				}
				if _, ok := err.(*CircularDependencyError); !ok {
					t.Errorf("Expected error of type *CircularDependencyError, but got %T", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Expected no error, but got %v", err)
			}

			names := make([]string, len(sorted))
			for i, n := range sorted {
				names[i] = n.Name
			}

			if !reflect.DeepEqual(names, tc.expectedOrder) {
				t.Errorf("Expected sorted order %v, but got %v", tc.expectedOrder, names)
			}
		})
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

func TestFilter(t *testing.T) {
	// A -> B -> C
	// D -> E

	testCases := []struct {
		name          string
		only          []string
		ignore        []string
		expectedNodes []string
		expectedError string
	}{
		{
			name:          "no filtering",
			expectedNodes: []string{"A", "B", "C", "D", "E", "root"},
		},
		{
			name:          "only A",
			only:          []string{"A"},
			expectedNodes: []string{"A", "B", "C"},
		},
		{
			name:          "only B",
			only:          []string{"B"},
			expectedNodes: []string{"B", "C"},
		},
		{
			name:          "only D",
			only:          []string{"D"},
			expectedNodes: []string{"D", "E"},
		},
		{
			name:          "ignore A",
			ignore:        []string{"A"},
			expectedNodes: []string{"D", "E", "root"},
		},
		{
			name:          "ignore B",
			ignore:        []string{"B"},
			expectedNodes: []string{"A", "D", "E", "root"},
		},
		{
			name:          "only A, ignore B",
			only:          []string{"A"},
			ignore:        []string{"B"},
			expectedNodes: []string{"A"},
		},
		{
			name:          "only A, ignore C",
			only:          []string{"A"},
			ignore:        []string{"C"},
			expectedNodes: []string{"A", "B"},
		},
		{
			name:          "no nodes left",
			only:          []string{"A"},
			ignore:        []string{"A"},
			expectedNodes: []string{"empty-root"},
		},
		{
			name:          "non-existent only",
			only:          []string{"X"},
			expectedError: "repository \"X\" not found in the graph",
		},
		{
			name:          "non-existent ignore",
			ignore:        []string{"X"},
			expectedError: "repository \"X\" not found in the graph",
		},
		{
			name:          "only with multiple roots",
			only:          []string{"B", "D"},
			expectedNodes: []string{"B", "C", "D", "E", "virtual-root"},
		},
		{
			name:          "ignore with multiple roots",
			ignore:        []string{"B", "D"},
			expectedNodes: []string{"A", "root"},
		},
		{
			name:          "only and ignore with multiple roots",
			only:          []string{"A", "D"},
			ignore:        []string{"B", "E"},
			expectedNodes: []string{"A", "D", "virtual-root"},
		},
		{
			name:          "filter results in cycle",
			only:          []string{"A", "B"},
			ignore:        []string{"C"},
			expectedNodes: []string{"A", "B"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a fresh graph for each test case
			nodeC := &Node{Name: "C", Path: "/C"}
			nodeB := &Node{Name: "B", Path: "/B", Children: []*Node{nodeC}}
			nodeA := &Node{Name: "A", Path: "/A", Children: []*Node{nodeB}}

			nodeE := &Node{Name: "E", Path: "/E"}
			nodeD := &Node{Name: "D", Path: "/D", Children: []*Node{nodeE}}

			rootNode := &Node{Name: "root", Path: "/", Children: []*Node{nodeA, nodeD}}

			// Create a cycle for the cycle test case
			if tc.name == "filter results in cycle" {
				nodeB.AddChild(nodeA)
			}

			filtered, err := rootNode.Filter(tc.only, tc.ignore)
			if tc.expectedError != "" {
				if err == nil {
					t.Fatal("Expected an error, but got nil")
				}
				if err.Error() != tc.expectedError {
					t.Errorf("Expected error '%s', but got '%s'", tc.expectedError, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("Expected no error, but got %v", err)
			}

			allNodes := filtered.AllNodes()
			var actualNames []string
			for _, node := range allNodes {
				actualNames = append(actualNames, node.Name)
			}

			assert.ElementsMatch(t, tc.expectedNodes, actualNames)
		})
	}
}
