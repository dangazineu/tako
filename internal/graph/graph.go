package graph

import (
	"fmt"
	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/git"
	"io"
	"path/filepath"
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

func BuildGraph(path, cacheDir string, localOnly bool) (*Node, error) {
	return buildGraphRecursive(path, cacheDir, make(map[string]*Node), []string{}, []string{}, localOnly)
}

func buildGraphRecursive(path, cacheDir string, visited map[string]*Node, currentPath []string, currentPathNames []string, localOnly bool) (*Node, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	for i, p := range currentPath {
		if p == absPath {
			cfg, err := config.Load(filepath.Join(absPath, "tako.yml"))
			if err != nil {
				// If we can't load the config, we can't get the name.
				// Just return the path with absPath.
				return nil, &CircularDependencyError{Path: append(currentPath, absPath)}
			}
			cyclePath := append(currentPathNames[i:], cfg.Metadata.Name)
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
		Name: cfg.Metadata.Name,
		Path: absPath,
	}

	newPath := append(currentPath, absPath)
	newPathNames := append(currentPathNames, root.Name)

	for _, dependent := range cfg.Dependents {
		repoPath, err := git.GetRepoPath(dependent.Repo, absPath, cacheDir, localOnly)
		if err != nil {
			return nil, err
		}

		child, err := buildGraphRecursive(repoPath, cacheDir, visited, newPath, newPathNames, localOnly)
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
