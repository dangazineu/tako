package graph

import (
	"fmt"
	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/git"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

type CircularDependencyError struct {
	Path []string
}

func (e *CircularDependencyError) Error() string {
	return fmt.Sprintf("circular dependency detected: %s", strings.Join(e.Path, " -> "))
}

type Node struct {
	Name     string
	Path     string
	Children []*Node
}

func (n *Node) AddChild(child *Node) {
	n.Children = append(n.Children, child)
}

func (n *Node) AllNodes() []*Node {
	var nodes []*Node
	visited := make(map[string]bool)
	var collect func(node *Node)
	collect = func(node *Node) {
		if visited[node.Name] {
			return
		}
		visited[node.Name] = true
		nodes = append(nodes, node)
		for _, child := range node.Children {
			collect(child)
		}
	}
	collect(n)
	return nodes
}

func (n *Node) TopologicalSort() ([]*Node, error) {
	inDegree := make(map[string]int)
	nodes := n.AllNodes()
	for _, node := range nodes {
		inDegree[node.Name] = 0
	}
	for _, node := range nodes {
		for _, child := range node.Children {
			inDegree[child.Name]++
		}
	}

	var queue []*Node
	for _, node := range nodes {
		if inDegree[node.Name] == 0 {
			queue = append(queue, node)
		}
	}
	sort.Slice(queue, func(i, j int) bool {
		return queue[i].Name < queue[j].Name
	})

	var sorted []*Node
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		sorted = append(sorted, node)

		sortedChildren := node.Children
		sort.Slice(sortedChildren, func(i, j int) bool {
			return sortedChildren[i].Name < sortedChildren[j].Name
		})

		for _, child := range sortedChildren {
			inDegree[child.Name]--
			if inDegree[child.Name] == 0 {
				queue = append(queue, child)
			}
		}
		sort.Slice(queue, func(i, j int) bool {
			return queue[i].Name < queue[j].Name
		})
	}

	if len(sorted) != len(nodes) {
		var cycle []string
		for _, node := range nodes {
			if inDegree[node.Name] > 0 {
				cycle = append(cycle, node.Name)
			}
		}
		return nil, &CircularDependencyError{Path: cycle}
	}

	return sorted, nil
}

func (n *Node) Filter(only, ignore []string) (*Node, error) {
	allNodes := n.AllNodes()
	nodeMap := make(map[string]*Node)
	for _, node := range allNodes {
		nodeMap[node.Name] = node
	}

	var onlyNodes []*Node
	if len(only) > 0 {
		for _, name := range only {
			if node, ok := nodeMap[name]; ok {
				onlyNodes = append(onlyNodes, node.AllNodes()...)
			} else {
				return nil, fmt.Errorf("repository %q not found in the graph", name)
			}
		}
	} else {
		onlyNodes = allNodes
	}

	var ignoreNodes []*Node
	if len(ignore) > 0 {
		for _, name := range ignore {
			if node, ok := nodeMap[name]; ok {
				ignoreNodes = append(ignoreNodes, node.AllNodes()...)
			} else {
				return nil, fmt.Errorf("repository %q not found in the graph", name)
			}
		}
	}

	filteredNodes := make(map[string]*Node)
	for _, node := range onlyNodes {
		isIgnored := false
		for _, ignoreNode := range ignoreNodes {
			if node.Name == ignoreNode.Name {
				isIgnored = true
				break
			}
		}
		if !isIgnored {
			// Create a new node with the same properties but empty children
			filteredNodes[node.Name] = &Node{
				Name:     node.Name,
				Path:     node.Path,
				Children: []*Node{},
			}
		}
	}

	// Reconnect children for the filtered nodes
	for _, originalNode := range onlyNodes {
		if filteredNode, ok := filteredNodes[originalNode.Name]; ok {
			for _, originalChild := range originalNode.Children {
				if _, childInFilteredSet := filteredNodes[originalChild.Name]; childInFilteredSet {
					filteredNode.AddChild(filteredNodes[originalChild.Name])
				}
			}
		}
	}

	// Find the root of the new graph. The root is the node with no parents in the filtered set.
	inDegree := make(map[string]int)
	for _, node := range filteredNodes {
		inDegree[node.Name] = 0
	}
	for _, node := range filteredNodes {
		for _, child := range node.Children {
			inDegree[child.Name]++
		}
	}

	var roots []*Node
	for name, degree := range inDegree {
		if degree == 0 {
			roots = append(roots, filteredNodes[name])
		}
	}

	if len(roots) == 0 && len(filteredNodes) > 0 {
		// This can happen if the filtering results in a graph with only cycles.
		// In this case, we can pick an arbitrary node as the root.
		for _, node := range filteredNodes {
			return node, nil
		}
	}
	if len(roots) == 1 {
		return roots[0], nil
	}

	// If there are multiple roots, create a new virtual root
	if len(roots) > 1 {
		newRoot := &Node{Name: "virtual-root", Path: ""}
		for _, root := range roots {
			newRoot.AddChild(root)
		}
		return newRoot, nil
	}

	// If there are no nodes left, return a new empty node
	return &Node{Name: "empty-root", Path: ""}, nil
}

func BuildGraph(repoName, path, cacheDir, homeDir string, localOnly bool) (*Node, error) {
	return buildGraphRecursive(repoName, path, cacheDir, homeDir, make(map[string]*Node), []string{}, []string{}, localOnly)
}

func buildGraphRecursive(repoName, path, cacheDir, homeDir string, visited map[string]*Node, currentPath []string, currentPathNames []string, localOnly bool) (*Node, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	for i, p := range currentPath {
		if p == absPath {
			cyclePath := append(currentPathNames[i:], repoName)
			return nil, &CircularDependencyError{Path: cyclePath}
		}
	}

	if node, ok := visited[absPath]; ok {
		return node, nil
	}

	cfg, err := config.Load(filepath.Join(absPath, "tako.yml"))
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	root := &Node{
		Name: repoName,
		Path: absPath,
	}

	newPath := append(currentPath, absPath)
	newPathNames := append(currentPathNames, root.Name)

	// Build graph from subscriptions (existing event-driven logic)
	for _, subscription := range cfg.Subscriptions {
		// subscription.Artifact is in the format "owner/repo:artifact"
		parts := strings.Split(subscription.Artifact, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid artifact reference in subscription: %s", subscription.Artifact)
		}
		depRepoName := parts[0]

		repoPath, err := git.GetRepoPath(depRepoName, absPath, cacheDir, homeDir, localOnly)
		if err != nil {
			return nil, err
		}

		child, err := buildGraphRecursive(depRepoName, repoPath, cacheDir, homeDir, visited, newPath, newPathNames, localOnly)
		if err != nil {
			return nil, err
		}
		root.AddChild(child)
	}

	// Build graph from dependents (new directed orchestration logic)
	for _, dependent := range cfg.Dependents {
		// dependent.Repo is in the format "owner/repo:branch"
		depRepoName := dependent.Repo

		repoPath, err := git.GetRepoPath(depRepoName, absPath, cacheDir, homeDir, localOnly)
		if err != nil {
			return nil, err
		}

		child, err := buildGraphRecursive(depRepoName, repoPath, cacheDir, homeDir, visited, newPath, newPathNames, localOnly)
		if err != nil {
			return nil, err
		}
		root.AddChild(child)
	}

	visited[absPath] = root

	return root, nil
}

func PrintGraph(w io.Writer, node *Node) {
	fmt.Fprintln(w, node.Name)
	printChildren(w, node.Children, "")
}

func printChildren(w io.Writer, children []*Node, prefix string) {
	for i, child := range children {
		isLast := i == len(children)-1
		line := prefix
		if isLast {
			line += "└── "
		} else {
			line += "├── "
		}
		fmt.Fprint(w, line+child.Name)

		if len(child.Children) > 0 {
			fmt.Fprintln(w)
			newPrefix := prefix
			if isLast {
				newPrefix += "    "
			} else {
				newPrefix += "│   "
			}
			printChildren(w, child.Children, newPrefix)
		} else {
			fmt.Fprintln(w)
		}
	}
}

func PrintDot(w io.Writer, root *Node) {
	fmt.Fprintln(w, "digraph {")
	printDotNode(w, root)
	fmt.Fprintln(w, "}")
}

func printDotNode(w io.Writer, node *Node) {
	fmt.Fprintf(w, "  %q [label=%q];\n", node.Name, node.Name)
	for _, child := range node.Children {
		fmt.Fprintf(w, "  %q -> %q;\n", node.Name, child.Name)
		printDotNode(w, child)
	}
}
