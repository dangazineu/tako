package graph

import (
	"fmt"
	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/git"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Node struct {
	Name     string
	Path     string
	Children []*Node
}

func (n *Node) AddChild(child *Node) {
	n.Children = append(n.Children, child)
}

func BuildGraph(path string, localOnly bool) (*Node, error) {
	return buildGraphRecursive(path, make(map[string]*Node), localOnly)
}

func buildGraphRecursive(path string, visited map[string]*Node, localOnly bool) (*Node, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
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
	visited[absPath] = root

	for _, dependent := range cfg.Dependents {
		repoPath, err := getRepoPath(dependent.Repo, absPath, localOnly)
		if err != nil {
			return nil, err
		}

		child, err := buildGraphRecursive(repoPath, visited, localOnly)
		if err != nil {
			// For now, just skip if the dependent is not found
			continue
		}
		root.AddChild(child)
	}

	return root, nil
}

var Clone = git.Clone

// getRepoPath resolves the local path to a dependent repository.
//
// If the repo path is relative (starts with "."), it is resolved relative to the
// current repository's path.
//
// If the repo path is a remote repository (e.g., "owner/repo:branch"), it is
// resolved to a standard location within the Tako cache
// (`~/.tako/cache/repos/owner/repo`).
//
// If the repository does not exist in the cache, it is cloned from GitHub. If it
// already exists, it is updated with a `git pull`.
func getRepoPath(repo string, currentPath string, localOnly bool) (string, error) {
	if strings.HasPrefix(repo, ".") {
		// Local relative path - always resolve relative to current path
		return filepath.Clean(filepath.Join(currentPath, strings.Split(repo, ":")[0])), nil
	}

	if strings.Contains(repo, "/") {
		// Remote repository reference (e.g., "tako-test/repo-y:main")
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home dir: %w", err)
		}

		repoParts := strings.Split(repo, "/")
		if len(repoParts) < 2 {
			return "", fmt.Errorf("invalid remote repository format: %s", repo)
		}
		repoOwner := repoParts[0]
		repoName := strings.Split(repoParts[1], ":")[0]
		repoPath := filepath.Join(homeDir, ".tako", "cache", "repos", repoOwner, repoName)

		if localOnly {
			// In local mode, only use if it exists in cache
			if _, err := os.Stat(repoPath); os.IsNotExist(err) {
				return "", fmt.Errorf("repository %s not found in cache (local mode)", repo)
			}
			return repoPath, nil
		} else {
			// In remote mode, clone/update as needed (current behavior)
			if _, err := os.Stat(repoPath); os.IsNotExist(err) {
				cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", repoOwner, repoName)
				if err := Clone(cloneURL, repoPath); err != nil {
					return "", err
				}
			} else {
				cmd := exec.Command("git", "-C", repoPath, "pull")
				if err := cmd.Run(); err != nil {
					return "", fmt.Errorf("failed to update repo %s: %w", repo, err)
				}
			}
			return repoPath, nil
		}
	}

	// Fallback for other patterns - treat as local relative path
	return filepath.Clean(filepath.Join(currentPath, strings.Split(repo, ":")[0])), nil
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
