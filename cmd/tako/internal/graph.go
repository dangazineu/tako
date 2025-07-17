package internal

import (
	"github.com/dangazineu/tako/internal/graph"
	"github.com/spf13/cobra"
	"os"
)

func NewGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Displays the dependency graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			rootPath, _ := cmd.Flags().GetString("root")
			if rootPath == "" {
				var err error
				rootPath, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			localOnly, _ := cmd.Flags().GetBool("local")
			cacheDir, _ := cmd.Flags().GetString("cache-dir")
			root, err := graph.BuildGraph(rootPath, cacheDir, localOnly)
			if err != nil {
				return err
			}
			graph.PrintGraph(cmd.OutOrStdout(), root)
			return nil
		},
	}
	cmd.Flags().String("root", "", "The root directory of the project")
	cmd.Flags().Bool("local", false, "Only use local repositories, do not clone or update remote repositories")
	return cmd
}
