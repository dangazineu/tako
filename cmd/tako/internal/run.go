package internal

import (
	"fmt"
	"github.com/dangazineu/tako/internal/git"
	"github.com/dangazineu/tako/internal/graph"
	"github.com/spf13/cobra"
	"os/exec"
	"strings"
)

func NewRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [command] [args...]",
		Short: "Execute a shell command across all dependent repositories",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := cmd.Flags().GetString("root")
			repo, _ := cmd.Flags().GetString("repo")
			local, _ := cmd.Flags().GetBool("local")
			cacheDir, _ := cmd.InheritedFlags().GetString("cache-dir")
			commandStr := strings.Join(args, " ")

			if strings.HasPrefix(commandStr, "mvn") {
				if _, err := exec.LookPath("mvn"); err != nil {
					return fmt.Errorf("mvn command not found, please install it to run this test")
				}
			}

			entrypointPath, err := git.GetEntrypointPath(root, repo, cacheDir, local)
			if err != nil {
				return err
			}

			rootNode, err := graph.BuildGraph(entrypointPath, cacheDir, local)
			if err != nil {
				return err
			}

			sortedNodes, err := rootNode.TopologicalSort()
			if err != nil {
				return err
			}

			//   1. Dependency: If repo-a's tako.yml lists repo-b as a dependent, it means B depends on A. The graph edge is A -> B.
			//   2. Build Order: To build B, its dependency A must be built first.
			//   3. Topological Sort: A topological sort of the graph A -> B will correctly produce the list [A, B].
			//   4. Conclusion: The run command must iterate through the topologically sorted list in its natural, forward order (A, then B) to ensure dependencies are built
			//      before the projects that need them.
			for _, node := range sortedNodes {
				fmt.Fprintf(cmd.OutOrStdout(), "--- Running in %s ---\n", node.Name)
				c := exec.Command("bash", "-c", commandStr)
				c.Dir = node.Path
				c.Stdout = cmd.OutOrStdout()
				c.Stderr = cmd.ErrOrStderr()
				if err := c.Run(); err != nil {
					return fmt.Errorf("command failed in %s: %w", node.Name, err)
				}
			}

			return nil
		},
	}
	cmd.Flags().String("root", "", "The root directory of the project")
	cmd.Flags().String("repo", "", "The remote repository to use as the entrypoint (e.g. owner/repo:ref)")
	cmd.Flags().Bool("local", false, "Only use local repositories, do not clone or update remote repositories")
	return cmd
}
