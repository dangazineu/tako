package git

import (
	"fmt"
	"github.com/dangazineu/tako/internal/errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Clone clones a repository from the given url into the given path.
func Clone(url, path string) error {
	var err error
	for i := 0; i < 3; i++ {
		cmd := exec.Command("git", "clone", url, path)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		err = errors.Wrap(err, "TAKO_E001", fmt.Sprintf("failed to clone repo %s: %s", url, string(output)))
		time.Sleep(2 * time.Second)
	}
	return err
}

// Checkout checks out a specific ref in the given repository path.
func Checkout(path, ref string) error {
	cmd := exec.Command("git", "-C", path, "checkout", ref)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, "TAKO_E002", fmt.Sprintf("failed to checkout ref %s in %s: %s", ref, path, string(output)))
	}
	return nil
}

func GetEntrypointPath(root, repo, cacheDir string, localOnly bool) (string, error) {
	if repo != "" {
		// When --repo is used, there's no "current" path, so we pass ""
		return GetRepoPath(repo, "", cacheDir, localOnly)
	}

	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return "", errors.Wrap(err, "TAKO_E003", "failed to get current working directory")
		}
	}
	return root, nil
}

// GetRepoPath resolves the local path to a dependent repository.
//
// If the repo path is relative (starts with "."), it is resolved relative to the
// current repository's path.
//
// If the repo path is a remote repository (e.g., "owner/repo:branch"), it is
// resolved to a standard location within the Tako cache
// (`~/.tako/cache/repos/owner/repo`).
//
// If the repository does not exist in the cache, it is cloned from GitHub. If it
// already exists, it is updated with a `git fetch`.
func GetRepoPath(repo, currentPath, cacheDir string, localOnly bool) (string, error) {
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
				return "", errors.Wrap(err, "TAKO_E004", "failed to get home dir")
			}
			cacheDir = filepath.Join(homeDir, ".tako", "cache")
		}

		repoParts := strings.Split(repo, "/")
		if len(repoParts) < 2 {
			return "", errors.New("TAKO_E005", fmt.Sprintf("invalid remote repository format: %s", repo))
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
			if currentPath != "" {
				localPath := filepath.Join(filepath.Dir(currentPath), repoName)
				if _, err := os.Stat(localPath); err == nil {
					repoPath = localPath
				}
			}
			if repoPath == "" {
				// Fallback to cache if not found in the immediate test structure
				repoPath = filepath.Join(cacheDir, "repos", repoOwner, repoName)
				if _, err := os.Stat(repoPath); os.IsNotExist(err) {
					return "", errors.Wrap(err, "TAKO_E006", fmt.Sprintf("repository %s not found in cache or local test structure", repo))
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
			} else {
				cmd := exec.Command("git", "-C", repoPath, "fetch")
				if err := cmd.Run(); err != nil {
					return "", errors.Wrap(err, "TAKO_E007", fmt.Sprintf("failed to update repo %s", repo))
				}
			}
			if ref != "" {
				if err := Checkout(repoPath, ref); err != nil {
					return "", err
				}
			}
		}
		return repoPath, nil
	}

	// Fallback for other patterns - treat as local relative path
	return filepath.Clean(filepath.Join(currentPath, strings.Split(repo, ":")[0])), nil
}
