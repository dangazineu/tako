package internal

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	var cacheDir string

	cmd := &cobra.Command{
		Use:   "tako",
		Short: "Tako is a command-line interface for multi-repository operations.",
		Long: `Tako is a command-line tool that simplifies multi-repository workflows by understanding the dependencies between your projects.
It allows you to run commands across your repositories in the correct order, ensuring that changes are built, tested, and released reliably.`,
	}

	cmd.PersistentFlags().StringVar(&cacheDir, "cache-dir", "~/.tako/cache", "The cache directory to use.")
	cmd.AddCommand(NewExecCmd())
	cmd.AddCommand(NewGraphCmd())
	cmd.AddCommand(NewRunCmd())
	cmd.AddCommand(NewCacheCmd())
	cmd.AddCommand(NewCompletionCmd())
	cmd.AddCommand(validateCmd)
	cmd.AddCommand(NewVersionCmd())

	return cmd
}

func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
