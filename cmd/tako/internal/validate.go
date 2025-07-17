package internal

import (
	"fmt"

	"github.com/dangazineu/tako/internal/config"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate [file]",
	Short: "Validate a tako.yml file",
	Long:  `Validate a tako.yml file.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := config.Load(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Validation successful!")
		return nil
	},
}
