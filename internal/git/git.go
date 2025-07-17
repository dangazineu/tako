package git

import (
	"fmt"
	"os/exec"
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
