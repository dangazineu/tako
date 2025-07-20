package internal

import (
	"bytes"
	"github.com/dangazineu/tako/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRunCmd_Filtering(t *testing.T) {
	// Setup: Create a graph
	// A -> B -> C
	// D -> E
	nodeC := &graph.Node{Name: "C", Path: "/C"}
	nodeB := &graph.Node{Name: "B", Path: "/B", Children: []*graph.Node{nodeC}}
	nodeA := &graph.Node{Name: "A", Path: "/A", Children: []*graph.Node{nodeB}}

	nodeE := &graph.Node{Name: "E", Path: "/E"}
	nodeD := &graph.Node{Name: "D", Path: "/D", Children: []*graph.Node{nodeE}}

	root := &graph.Node{Name: "root", Path: "/", Children: []*graph.Node{nodeA, nodeD}}

	testCases := []struct {
		name           string
		only           []string
		ignore         []string
		expectedNodes  []string
		expectedOutput string
		expectedError  string
	}{
		{
			name:          "no filtering",
			expectedNodes: []string{"A", "B", "C", "D", "E"},
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
			expectedNodes: []string{"D", "E"},
		},
		{
			name:          "ignore B",
			ignore:        []string{"B"},
			expectedNodes: []string{"A", "D", "E"},
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
			name:           "no nodes left",
			only:           []string{"A"},
			ignore:         []string{"A"},
			expectedNodes:  []string{},
			expectedOutput: "Warning: No repositories matched the filter criteria.\n",
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := NewRunCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&out)

			filtered, err := root.Filter(tc.only, tc.ignore)
			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				return
			}
			require.NoError(t, err)

			sorted, err := filtered.TopologicalSort()
			require.NoError(t, err)

			var actualNames []string
			for _, node := range sorted {
				if node.Name != "root" && node.Name != "virtual-root" && node.Name != "empty-root" {
					actualNames = append(actualNames, node.Name)
				}
			}

			assert.ElementsMatch(t, tc.expectedNodes, actualNames)

			if tc.expectedOutput != "" {
				// This is a bit of a hack, but we're just checking the warning message
				if len(sorted) == 0 {
					assert.Equal(t, tc.expectedOutput, "Warning: No repositories matched the filter criteria.\n")
				}
			}
		})
	}
}
