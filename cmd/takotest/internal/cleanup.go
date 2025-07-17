package internal

import (
	"fmt"
	"github.com/dangazineu/tako/test/e2e"
	"github.com/spf13/cobra"
)

func NewCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup [testcase]",
		Short: "Cleanup a test case",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			testCaseName := args[0]
			testCase, ok := e2e.TestCases[testCaseName]
			if !ok {
				return fmt.Errorf("test case not found: %s", testCaseName)
			}

			client, err := e2e.GetClient()
			if err != nil {
				return err
			}

			fmt.Printf("Cleaning up test case: %s\n", testCaseName)
			return testCase.Cleanup(client)
		},
	}
	return cmd
}
