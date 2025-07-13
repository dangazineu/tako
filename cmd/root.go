package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "tako",
	Short: "Tako is a command-line interface for multi-repository operations.",
	Long: `Tako is a command-line tool that simplifies multi-repository workflows by understanding the dependencies between your projects.
It allows you to run commands across your repositories in the correct order, ensuring that changes are built, tested, and released reliably.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
