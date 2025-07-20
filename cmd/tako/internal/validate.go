package internal

import (
	"fmt"
	"path/filepath"

	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/git"
	"github.com/spf13/cobra"
)

func NewValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a tako.yml file",
		Long:  `Validate a tako.yml file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := cmd.Flags().GetString("root")
			repo, _ := cmd.Flags().GetString("repo")
			local, _ := cmd.Flags().GetBool("local")
			cacheDir, _ := cmd.InheritedFlags().GetString("cache-dir")

			entrypointPath, err := git.GetEntrypointPath(root, repo, cacheDir, local)
			if err != nil {
				return err
			}

			_, err = config.Load(filepath.Join(entrypointPath, "tako.yml"))
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Validation successful!")
			return nil
		},
	}
	cmd.Flags().String("root", "", "The root directory of the project")
	cmd.Flags().String("repo", "", "The remote repository to use as the entrypoint (e.g. owner/repo:ref)")
	cmd.Flags().Bool("local", false, "Only use local repositories, do not clone or update remote repositories")
	return cmd
}

var validateCmd = NewValidateCmd()
