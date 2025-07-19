package internal

import (
	"fmt"
	"github.com/dangazineu/tako/test/e2e"
	"github.com/spf13/cobra"
)

func NewSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup [testcase]",
		Short: "Setup a test case",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			local, _ := cmd.Flags().GetBool("local")
			withRepoEntrypoint, _ := cmd.Flags().GetBool("with-repo-entrypoint")
			owner, _ := cmd.Flags().GetString("owner")
			testCaseName := args[0]
			testCase, ok := e2e.GetTestCases(owner)[testCaseName]
			if !ok {
				return fmt.Errorf("test case not found: %s", testCaseName)
			}
			testCase.WithRepoEntryPoint = withRepoEntrypoint

			if local {
				fmt.Printf("Setting up local test case\n")
				testCaseDir, err := testCase.SetupLocal()
				if err != nil {
					return err
				}
				fmt.Printf("Test case set up in: %s\n", testCaseDir)
				return nil
			}

			client, err := e2e.GetClient()
			if err != nil {
				return err
			}

			fmt.Printf("Setting up remote test case: %s\n", testCaseName)
			return testCase.Setup(client)
		},
	}
	cmd.Flags().Bool("local", false, "Setup the test case locally")
	cmd.Flags().Bool("with-repo-entrypoint", false, "Setup the test case with a remote entrypoint")
	cmd.Flags().String("owner", "", "The owner of the repositories")
	cmd.MarkFlagRequired("owner")
	return cmd
}
