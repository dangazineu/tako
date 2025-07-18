package graph

import (
	"fmt"
	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/git"
	"io"
	"os"
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
		repoPath, err := getRepoPath(dependent.Repo, absPath, cacheDir, localOnly)
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

var Clone = git.Clone

// getRepoPath resolves the local path to a dependent repository.
// It expects the repo in the format "owner/repo:branch" and resolves it
// to a standard location within the Tako cache (`~/.tako/cache/repos/owner/repo`).
//
// If the repository does not exist in the cache, it is cloned from GitHub. If it
// already exists, it is updated.
func getRepoPath(repo, currentPath, cacheDir string, localOnly bool) (string, error) {
	if !strings.Contains(repo, "/") {
		return "", fmt.Errorf("invalid remote repository format: %s", repo)
	}

	// Remote repository reference (e.g., "tako-test/repo-y:main")
	if cacheDir == "~/.tako/cache" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home dir: %w", err)
		}
		cacheDir = filepath.Join(homeDir, ".tako", "cache")
	}

	repoParts := strings.Split(repo, "/")
	if len(repoParts) < 2 {
		return "", fmt.Errorf("invalid remote repository format: %s", repo)
	}
	repoOwner := repoParts[0]

	repoAndRef := strings.Split(repoParts[1], ":")
	repoName := repoAndRef[0]
	var ref string
	if len(repoAndRef) > 1 {
		ref = repoAndRef[1]
	}

	var repoPath string

	if localOnly {
		// In local mode, we still use the cache, but we don't clone/update.
		// This is to support E2E tests that pre-populate the cache.
		repoPath = filepath.Join(cacheDir, "repos", repoOwner, repoName)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			return "", fmt.Errorf("repository %s not found in cache", repo)
		}
	} else {
		// In remote mode, always use the cache
		repoPath = filepath.Join(cacheDir, "repos", repoOwner, repoName)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", repoOwner, repoName)
			if err := Clone(cloneURL, repoPath); err != nil {
				return "", err
			}
		}

		if ref == "" {
			var err error
			ref, err = git.GetDefaultBranch(repoPath)
			if err != nil {
				return "", err
			}
		}

		if err := git.Checkout(repoPath, ref); err != nil {
			return "", err
		}
	}
	return repoPath, nil
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
