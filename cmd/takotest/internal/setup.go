package internal

import (
	"fmt"
	"github.com/dangazineu/tako/test/e2e"
	"github.com/spf13/cobra"
	"os"
)

func NewSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup [testcase]",
		Short: "Setup a test case",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			local, _ := cmd.Flags().GetBool("local")
			testCaseName := args[0]
			testCase, ok := e2e.TestCases[testCaseName]
			if !ok {
				return fmt.Errorf("test case not found: %s", testCaseName)
			}

			if local {
				tmpDir, err := os.MkdirTemp("", "tako-test")
				if err != nil {
					return err
				}
				fmt.Printf("Setting up local test case in: %s\n", tmpDir)
				return testCase.SetupLocal(tmpDir)
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
	return cmd
}
