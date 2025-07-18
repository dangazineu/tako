package internal

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func NewCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage tako's cache",
	}

	cmd.AddCommand(newCacheCleanCmd())

	return cmd
}

func newCacheCleanCmd() *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clear the cache directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			cacheDir, err := cmd.Flags().GetString("cache-dir")
			if err != nil {
				return err
			}

			if cacheDir == "~/.tako/cache" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				cacheDir = filepath.Join(homeDir, ".tako", "cache")
			}

			if !confirm {
				cmd.OutOrStdout().Write([]byte("This will delete the cache directory at " + cacheDir + ". Use --confirm to proceed.\n"))
				return nil
			}

			cmd.OutOrStdout().Write([]byte("Cleaning cache...\n"))
			if err := os.RemoveAll(cacheDir); err != nil {
				return err
			}
			cmd.OutOrStdout().Write([]byte("Cache cleaned successfully.\n"))

			return nil
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm the cache cleaning")
	return cmd
}
