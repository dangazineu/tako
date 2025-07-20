package internal

import (
	"context"
	"fmt"
	"github.com/dangazineu/tako/test/e2e"
	"github.com/google/go-github/v63/github"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

func NewCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup [environment]",
		Short: "Cleanup a test environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			local, _ := cmd.Flags().GetBool("local")
			owner, _ := cmd.Flags().GetString("owner")
			envName := args[0]
			env, ok := e2e.GetEnvironments(owner)[envName]
			if !ok {
				return fmt.Errorf("environment not found: %s", envName)
			}

			if local {
				tmpDir := filepath.Join(os.TempDir(), env.Name)
				return os.RemoveAll(tmpDir)
			}

			client, err := e2e.GetClient()
			if err != nil {
				return err
			}
			for _, repoDef := range env.Repositories {
				repoName := fmt.Sprintf("%s-%s", env.Name, repoDef.Name)
				_, err := client.Repositories.Delete(context.Background(), owner, repoName)
				if err != nil {
					if _, ok := err.(*github.ErrorResponse); !ok || err.(*github.ErrorResponse).Response.StatusCode != 404 {
						return err
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().String("owner", "", "The owner of the repositories")
	cmd.Flags().Bool("local", false, "Cleanup the test case locally")
	cmd.MarkFlagRequired("owner")
	return cmd
}
