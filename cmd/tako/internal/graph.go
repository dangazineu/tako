package internal

import (
	"github.com/dangazineu/tako/internal/git"
	"github.com/dangazineu/tako/internal/graph"
	"github.com/spf13/cobra"
	"os"
)

func NewGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Displays the dependency graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := cmd.Flags().GetString("root")
			repo, _ := cmd.Flags().GetString("repo")
			local, _ := cmd.Flags().GetBool("local")
			dot, _ := cmd.Flags().GetBool("dot")
			cacheDir, _ := cmd.InheritedFlags().GetString("cache-dir")

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
				if _, ok := err.(*graph.CircularDependencyError); ok {
					return err
				}
				return err
			}

			if dot {
				graph.PrintDot(cmd.OutOrStdout(), rootNode)
			} else {
				graph.PrintGraph(cmd.OutOrStdout(), rootNode)
			}
			return nil
		},
	}
	cmd.Flags().String("root", "", "The root directory of the project")
	cmd.Flags().String("repo", "", "The remote repository to use as the entrypoint (e.g. owner/repo:ref)")
	cmd.Flags().Bool("local", false, "Only use local repositories, do not clone or update remote repositories")
	cmd.Flags().Bool("dot", false, "Output the graph in DOT format")
	return cmd
}
