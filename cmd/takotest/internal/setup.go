package internal

import (
	"encoding/json"
	"fmt"
	"github.com/dangazineu/tako/internal/git"
	"github.com/dangazineu/tako/test/e2e"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

type SetupOutput struct {
	WorkDir  string `json:"workDir"`
	CacheDir string `json:"cacheDir"`
}

func NewSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup [testcase]",
		Short: "Setup a test case",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			local, _ := cmd.Flags().GetBool("local")
			withRepoEntrypoint, _ := cmd.Flags().GetBool("with-repo-entrypoint")
			owner, _ := cmd.Flags().GetString("owner")
			testCaseName := args[0]
			testCase, ok := e2e.GetTestCases(owner)[testCaseName]
			if !ok {
				return fmt.Errorf("test case not found: %s", testCaseName)
			}

			var workDir, cacheDir string
			if local {
				testCaseDir, err := testCase.SetupLocal(withRepoEntrypoint)
				if err != nil {
					return err
				}
				workDir = filepath.Join(testCaseDir, "workdir")
				cacheDir = filepath.Join(testCaseDir, "cache")
			} else {
				client, err := e2e.GetClient()
				if err != nil {
					return err
				}
				if err := testCase.Setup(client); err != nil {
					return err
				}
				tmpDir, err := os.MkdirTemp("", "tako-e2e-")
				if err != nil {
					return err
				}
				workDir = tmpDir
				cacheDir = filepath.Join(tmpDir, "cache")
				if !withRepoEntrypoint {
					if err := git.Clone(testCase.Repositories[0].CloneURL, workDir); err != nil {
						return err
					}
				}
			}

			output := SetupOutput{
				WorkDir:  workDir,
				CacheDir: cacheDir,
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
		},
	}
	cmd.Flags().Bool("local", false, "Setup the test case locally")
	cmd.Flags().Bool("with-repo-entrypoint", false, "Setup the test case with a remote entrypoint")
	cmd.Flags().String("owner", "", "The owner of the repositories")
	cmd.MarkFlagRequired("owner")
	return cmd
}