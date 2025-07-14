package internal

import (
	"fmt"
	"github.com/spf13/cobra"
)

func NewCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Cleanup a test case",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Cleanup is a no-op. Please manually delete the test directory.")
			return nil
		},
	}
	return cmd
}
