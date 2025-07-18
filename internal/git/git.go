package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// Clone clones a repository from the given url into the given path.
func Clone(url, path string) error {
	cmd := exec.Command("git", "clone", url, path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to clone repo %s: %s", url, string(output))
	}
	return nil
}

// Checkout checks out the given ref in the given path.
func Checkout(path, ref string) error {
	// Fetch all tags and branches from the remote
	cmd := exec.Command("git", "-C", path, "fetch", "origin", "--force")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to fetch from origin in repo %s: %s", path, string(output))
	}

	// Check if the ref is a branch
	cmd = exec.Command("git", "-C", path, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+ref)
	isBranch := cmd.Run() == nil

	if isBranch {
		// If it's a branch, we want to check it out and pull the latest changes
		cmd = exec.Command("git", "-C", path, "checkout", ref)
		output, err = cmd.CombinedOutput()
		if err != nil {
			// It might be that the local branch is not tracking the remote one.
			// Let's try to set it up.
			cmd = exec.Command("git", "-C", path, "checkout", "-b", ref, "origin/"+ref)
			output, err = cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to checkout branch %s in repo %s: %s", ref, path, string(output))
			}
		}

		cmd = exec.Command("git", "-C", path, "pull", "origin", ref)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to pull branch %s in repo %s: %s", ref, path, string(output))
		}
	} else {
		// If it's not a branch, it could be a tag or a commit hash.
		// In this case, we just check it out. This will result in a detached HEAD.
		cmd = exec.Command("git", "-C", path, "checkout", ref)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to checkout ref %s in repo %s: %s", ref, path, string(output))
		}
	}

	return nil
}

// Pull updates the current branch in the given path.
func Pull(path string) error {
	cmd := exec.Command("git", "-C", path, "pull")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to pull in repo %s: %s", path, string(output))
	}
	return nil
}

// GetDefaultBranch returns the default branch of the repository at the given path.
func GetDefaultBranch(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "origin/HEAD")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get default branch for repo %s: %s", path, string(output))
	}
	return strings.TrimSpace(strings.TrimPrefix(string(output), "origin/")), nil
}
