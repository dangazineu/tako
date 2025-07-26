package internal

import (
	"fmt"
	"os"
	"strings"

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

			// TODO: Implement the actual execution logic.
			return nil
		},
	}

	cmd.Flags().String("repo", "", "Specify the repository to run the workflow in (e.g., my-org/my-repo)")
	cmd.Flags().String("resume", "", "Resume a previous workflow execution by providing the run ID")
	cmd.Flags().StringToString("inputs", nil, "Pass input variables to the workflow (e.g., --inputs.version-bump=minor)")
	cmd.Flags().Bool("dry-run", false, "Show the execution plan without making any changes")
	cmd.Flags().Bool("no-cache", false, "Invalidate the cache and execute all steps")
	cmd.Flags().Int("max-concurrent-repos", 4, "Maximum number of repositories to process in parallel")
	cmd.Flags().Bool("debug", false, "Enable interactive step-by-step execution")
	cmd.FParseErrWhitelist.UnknownFlags = true

	return cmd
}
