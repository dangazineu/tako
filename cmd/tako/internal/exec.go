package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dangazineu/tako/internal/engine"
	"github.com/spf13/cobra"
)

func NewExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec <workflow-name>",
		Short: "Execute a workflow",
		Long: `Executes a workflow defined in the tako.yml file.
You can specify a workflow by its name.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowName := args[0]
			repo, _ := cmd.Flags().GetString("repo")
			resume, _ := cmd.Flags().GetString("resume")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			debug, _ := cmd.Flags().GetBool("debug")
			noCache, _ := cmd.Flags().GetBool("no-cache")
			maxConcurrentRepos, _ := cmd.Flags().GetInt("max-concurrent-repos")
			localOnly, _ := cmd.Flags().GetBool("local")

			// Get cache directory
			cacheDir, _ := cmd.Flags().GetString("cache-dir")
			if cacheDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get user home directory: %v", err)
				}
				cacheDir = filepath.Join(homeDir, ".tako", "cache")
			}

			inputs := make(map[string]string)
			for _, arg := range os.Args {
				if strings.HasPrefix(arg, "--inputs.") {
					parts := strings.SplitN(strings.TrimPrefix(arg, "--inputs."), "=", 2)
					if len(parts) == 2 {
						inputs[parts[0]] = parts[1]
					}
				}
			}

			fmt.Printf("Executing workflow '%s'\n", workflowName)
			if repo != "" {
				fmt.Printf("Repository: %s\n", repo)
			}
			if resume != "" {
				fmt.Printf("Resuming from: %s\n", resume)
			}
			if len(inputs) > 0 {
				fmt.Println("Inputs:")
				for k, v := range inputs {
					fmt.Printf("  %s: %s\n", k, v)
				}
			}

			// Handle resume operation
			if resume != "" {
				return handleResumeExecution(resume, cacheDir)
			}

			// Determine workspace root
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get user home directory: %v", err)
			}
			workspaceRoot := filepath.Join(homeDir, ".tako", "workspaces")

			// Create execution runner
			runnerOpts := engine.RunnerOptions{
				WorkspaceRoot:      workspaceRoot,
				CacheDir:           cacheDir,
				MaxConcurrentRepos: maxConcurrentRepos,
				DryRun:             dryRun,
				Debug:              debug,
				NoCache:            noCache,
				Environment:        os.Environ(),
			}

			runner, err := engine.NewRunner(runnerOpts)
			if err != nil {
				return fmt.Errorf("failed to create execution runner: %v", err)
			}
			defer runner.Close()

			ctx := context.Background()

			if repo != "" {
				// Multi-repository execution mode
				result, err := runner.ExecuteMultiRepoWorkflow(ctx, workflowName, inputs, repo, localOnly)
				if err != nil {
					// Still print the result if available, even when there's an error
					if result != nil {
						printErr := printExecutionResult(result)
						if printErr != nil && printErr.Error() == "execution failed" {
							// The workflow execution failed (expected for some tests)
							// Return detailed error message that includes both "execution failed" and the specific step error
							if result.Error != nil {
								return fmt.Errorf("execution failed: %v", result.Error)
							}
							return printErr
						}
					}
					return fmt.Errorf("multi-repository execution failed: %v", err)
				}
				return printExecutionResult(result)
			} else {
				// Single-repository execution mode
				repoPath, err := determineRepositoryPath(cmd)
				if err != nil {
					return fmt.Errorf("failed to determine repository path: %v", err)
				}

				result, err := runner.ExecuteWorkflow(ctx, workflowName, inputs, repoPath)
				if err != nil {
					return fmt.Errorf("workflow execution failed: %v", err)
				}
				return printExecutionResult(result)
			}
		},
	}

	cmd.Flags().String("repo", "", "Specify the repository to run the workflow in (e.g., my-org/my-repo)")
	cmd.Flags().String("resume", "", "Resume a previous workflow execution by providing the run ID")
	cmd.Flags().StringToString("inputs", nil, "Pass input variables to the workflow (e.g., --inputs.version-bump=minor)")
	cmd.Flags().Bool("dry-run", false, "Show the execution plan without making any changes")
	cmd.Flags().Bool("no-cache", false, "Invalidate the cache and execute all steps")
	cmd.Flags().Int("max-concurrent-repos", 4, "Maximum number of repositories to process in parallel")
	cmd.Flags().Bool("debug", false, "Enable interactive step-by-step execution")
	cmd.Flags().Bool("local", false, "Run in local mode without network access")
	cmd.Flags().String("cache-dir", "", "Directory for caching repositories (default: ~/.tako/cache)")
	cmd.Flags().String("root", "", "Root directory for local repository execution")
	cmd.FParseErrWhitelist.UnknownFlags = true

	return cmd
}

// handleResumeExecution handles resuming a previous execution.
func handleResumeExecution(runID, cacheDir string) error {
	// TODO: Implement resume functionality
	fmt.Printf("Resuming execution %s...\n", runID)
	return fmt.Errorf("resume functionality not yet implemented")
}

// determineRepositoryPath determines the repository path for execution.
func determineRepositoryPath(cmd *cobra.Command) (string, error) {
	// Check for --root flag first
	rootPath, _ := cmd.Flags().GetString("root")
	if rootPath != "" {
		return rootPath, nil
	}

	// Fall back to current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %v", err)
	}

	return cwd, nil
}

// printExecutionResult prints the execution result.
func printExecutionResult(result *engine.ExecutionResult) error {
	if result == nil {
		return fmt.Errorf("no execution result")
	}

	fmt.Printf("\nExecution completed: %s\n", result.RunID)
	fmt.Printf("Success: %v\n", result.Success)
	fmt.Printf("Duration: %v\n", result.EndTime.Sub(result.StartTime))

	if result.Error != nil {
		fmt.Printf("Error: %v\n", result.Error)
	}

	if len(result.Steps) > 0 {
		fmt.Printf("\nSteps executed: %d\n", len(result.Steps))
		for _, step := range result.Steps {
			status := "✓"
			if !step.Success {
				status = "✗"
			}
			fmt.Printf("  %s %s (%v)\n", status, step.ID, step.EndTime.Sub(step.StartTime))
		}
	}

	if !result.Success {
		return fmt.Errorf("execution failed")
	}

	return nil
}
