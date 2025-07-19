package internal

import (
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

func NewCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage tako's cache",
	}

	cmd.AddCommand(newCacheCleanCmd())
	cmd.AddCommand(newCachePruneCmd())

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

func newCachePruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Prune the cache directory",
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

			cmd.OutOrStdout().Write([]byte("Pruning cache...\n"))
			if err := CleanOld(cacheDir, 30*24*time.Hour); err != nil {
				return err
			}
			cmd.OutOrStdout().Write([]byte("Cache pruned successfully.\n"))

			return nil
		},
	}
	return cmd
}

func CleanOld(cacheDir string, maxAge time.Duration) error {
	reposDir := filepath.Join(cacheDir, "repos")
	return filepath.Walk(reposDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && path != reposDir {
			if time.Since(info.ModTime()) > maxAge {
				return os.RemoveAll(path)
			}
		}
		return nil
	})
}