package internal

import (
	"fmt"
	"github.com/dangazineu/tako/internal/git"
	"github.com/dangazineu/tako/internal/graph"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
)

func NewGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Displays the dependency graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, _ := cmd.Flags().GetString("repo")
			rootPath, _ := cmd.Flags().GetString("root")
			cacheDir, _ := cmd.Flags().GetString("cache-dir")

			if repo != "" {
				parts := strings.Split(repo, ":")
				if len(parts) != 2 {
					return fmt.Errorf("invalid repo format, expected owner/repo:ref")
				}
				repoParts := strings.Split(parts[0], "/")
				if len(repoParts) != 2 {
					return fmt.Errorf("invalid repo format, expected owner/repo:ref")
				}

				owner := repoParts[0]
				repoName := repoParts[1]
				ref := parts[1]

				repoCacheDir := filepath.Join(cacheDir, "repos", owner, repoName)
				if _, err := os.Stat(repoCacheDir); os.IsNotExist(err) {
					if err := git.Clone(fmt.Sprintf("https://github.com/%s/%s.git", owner, repoName), repoCacheDir); err != nil {
						return err
					}
				}

				if err := git.Checkout(repoCacheDir, ref); err != nil {
					return err
				}

				rootPath = repoCacheDir
			} else if rootPath == "" {
				var err error
				rootPath, err = os.Getwd()
				if err != nil {
					return err
				}
			}

			localOnly, _ := cmd.Flags().GetBool("local")
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
	cmd.Flags().String("repo", "", "The remote repository to use as the entrypoint (e.g. owner/repo:ref)")

	return cmd
}
