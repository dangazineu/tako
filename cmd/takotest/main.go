package main

import (
	"fmt"
	"os"

	"github.com/dangazineu/tako/cmd/takotest/internal"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "takotest",
	Short: "A tool for managing e2e tests for tako",
}

func main() {
	rootCmd.AddCommand(internal.NewSetupCmd())
	rootCmd.AddCommand(internal.NewCleanupCmd())
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
