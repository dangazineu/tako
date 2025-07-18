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

type Node struct {
	Name     string
	Path     string
	Children []*Node
}

func (n *Node) AddChild(child *Node) {
	n.Children = append(n.Children, child)
}

func BuildGraph(path, cacheDir string, localOnly bool) (*Node, error) {
	return buildGraphRecursive(path, cacheDir, make(map[string]*Node), localOnly)
}

func buildGraphRecursive(path, cacheDir string, visited map[string]*Node, localOnly bool) (*Node, error) {
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
		repoPath, err := getRepoPath(dependent.Repo, absPath, cacheDir, localOnly)
		if err != nil {
			return nil, err
		}

		child, err := buildGraphRecursive(repoPath, cacheDir, visited, localOnly)
		if err != nil {
			return nil, err
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
func getRepoPath(repo, currentPath, cacheDir string, localOnly bool) (string, error) {
	if strings.HasPrefix(repo, "file://") {
		return strings.Split(strings.TrimPrefix(repo, "file://"), ":")[0], nil
	}
	if strings.HasPrefix(repo, ".") {
		// Local relative path - always resolve relative to current path
		return filepath.Clean(filepath.Join(currentPath, strings.Split(repo, ":")[0])), nil
	}

	if strings.Contains(repo, "/") {
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
			// In local mode, try to resolve from the parent directory first
			// to support nested E2E test structures.
			localPath := filepath.Join(filepath.Dir(currentPath), repoName)
			if _, err := os.Stat(localPath); err == nil {
				repoPath = localPath
			} else {
				// Fallback to cache if not found in the immediate test structure
				repoPath = filepath.Join(cacheDir, "repos", repoOwner, repoName)
				if _, err := os.Stat(repoPath); os.IsNotExist(err) {
					return "", fmt.Errorf("repository %s not found in cache or local test structure", repo)
				}
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
