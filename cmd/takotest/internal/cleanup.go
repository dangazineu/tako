package internal

import (
	"fmt"
	"github.com/dangazineu/tako/test/e2e"
	"github.com/spf13/cobra"
	"os"
)

func NewCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup [testcase]",
		Short: "Cleanup a test case",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			local, _ := cmd.Flags().GetBool("local")
			owner, _ := cmd.Flags().GetString("owner")
			workDir, _ := cmd.Flags().GetString("work-dir")
			cacheDir, _ := cmd.Flags().GetString("cache-dir")
			testCaseName := args[0]
			testCase, ok := e2e.GetTestCases(owner)[testCaseName]
			if !ok {
				return fmt.Errorf("test case not found: %s", testCaseName)
			}

			if workDir != "" {
				if err := os.RemoveAll(workDir); err != nil {
					return err
				}
			}
			if cacheDir != "" {
				if err := os.RemoveAll(cacheDir); err != nil {
					return err
				}
			}

			if !local {
				client, err := e2e.GetClient()
				if err != nil {
					return err
				}
				return testCase.Cleanup(client)
			}
			return nil
		},
	}
	cmd.Flags().String("owner", "", "The owner of the repositories")
	cmd.Flags().String("work-dir", "", "The working directory to cleanup")
	cmd.Flags().String("cache-dir", "", "The cache directory to cleanup")
	cmd.Flags().Bool("local", false, "Cleanup the test case locally")
	cmd.MarkFlagRequired("owner")
	return cmd
}
