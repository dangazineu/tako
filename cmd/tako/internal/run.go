package internal

import (
	"fmt"
	"github.com/dangazineu/tako/internal/git"
	"github.com/dangazineu/tako/internal/graph"
	"github.com/spf13/cobra"
	"os"
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
			only, _ := cmd.Flags().GetStringSlice("only")
			ignore, _ := cmd.Flags().GetStringSlice("ignore")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			cacheDir, _ := cmd.InheritedFlags().GetString("cache-dir")
			commandStr := strings.Join(args, " ")

			if strings.HasPrefix(commandStr, "mvn") {
				if _, err := exec.LookPath("mvn"); err != nil {
					return fmt.Errorf("mvn command not found, please install it to run this test")
				}
			}

			workingDir, err := os.Getwd()
			if err != nil {
				return err
			}
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return err
			}

			entrypointPath, err := git.GetEntrypointPath(root, repo, cacheDir, workingDir, homeDir, local)
			if err != nil {
				return err
			}

			rootNode, err := graph.BuildGraph(entrypointPath, cacheDir, homeDir, local)
			if err != nil {
				return err
			}

			filteredNodes, err := rootNode.Filter(only, ignore)
			if err != nil {
				return err
			}

			sortedNodes, err := filteredNodes.TopologicalSort()
			if err != nil {
				return err
			}

			if len(sortedNodes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Warning: No repositories matched the filter criteria.")
				return nil
			}

			//   1. Dependency: If repo-a's tako.yml lists repo-b as a dependent, it means B depends on A. The graph edge is A -> B.
			//   2. Build Order: To build B, its dependency A must be built first.
			//   3. Topological Sort: A topological sort of the graph A -> B will correctly produce the list [A, B].
			//   4. Conclusion: The run command must iterate through the topologically sorted list in its natural, forward order (A, then B) to ensure dependencies are built
			//      before the projects that need them.
			for _, node := range sortedNodes {
				if dryRun {
					fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] %s: %s\n", node.Name, commandStr)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "--- Running in %s ---\n", node.Name)
					c := exec.Command("bash", "-c", commandStr)
					c.Dir = node.Path
					c.Stdout = cmd.OutOrStdout()
					c.Stderr = cmd.ErrOrStderr()
					if err := c.Run(); err != nil {
						return fmt.Errorf("command failed in %s: %w", node.Name, err)
					}
				}
			}

			return nil
		},
	}
	cmd.Flags().String("root", "", "The root directory of the project")
	cmd.Flags().String("repo", "", "The remote repository to use as the entrypoint (e.g. owner/repo:ref)")
	cmd.Flags().Bool("local", false, "Only use local repositories, do not clone or update remote repositories")
	cmd.Flags().StringSlice("only", []string{}, "Only run on the specified repository and its dependents")
	cmd.Flags().StringSlice("ignore", []string{}, "Ignore the specified repository and its dependents")
	cmd.Flags().Bool("dry-run", false, "Show what commands would be run without executing them")
	return cmd
}
