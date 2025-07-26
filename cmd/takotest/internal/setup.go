package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/git"
	"github.com/dangazineu/tako/test/e2e"
	"github.com/google/go-github/v63/github"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type SetupOutput struct {
	WorkDir  string `json:"workDir"`
	CacheDir string `json:"cacheDir"`
}

func NewSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup [environment]",
		Short: "Setup a test environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			local, _ := cmd.Flags().GetBool("local")
			owner, _ := cmd.Flags().GetString("owner")
			envName := args[0]
			env, ok := e2e.GetEnvironments(owner)[envName]
			if !ok {
				return fmt.Errorf("environment not found: %s", envName)
			}

			if local {
				return setupLocal(cmd, &env, owner)
			}
			return setupRemote(cmd, &env, owner)
		},
	}
	cmd.Flags().Bool("local", false, "Setup the test case locally")
	cmd.Flags().String("work-dir", "", "The working directory to use")
	cmd.Flags().String("cache-dir", "", "The cache directory to use")
	cmd.Flags().Bool("with-repo-entrypoint", false, "Setup the test case with a remote entrypoint")
	cmd.Flags().String("owner", "", "The owner of the repositories")
	cmd.MarkFlagRequired("owner")
	return cmd
}

func setupLocal(cmd *cobra.Command, env *e2e.TestEnvironmentDef, owner string) error {
	withRepoEntrypoint, _ := cmd.Flags().GetBool("with-repo-entrypoint")
	workDir, _ := cmd.Flags().GetString("work-dir")
	cacheDir, _ := cmd.Flags().GetString("cache-dir")

	if workDir == "" {
		tmpDir := filepath.Join(os.TempDir(), env.Name)
		if err := os.RemoveAll(tmpDir); err != nil {
			return err
		}
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			return err
		}
		workDir = filepath.Join(tmpDir, "workdir")
		cacheDir = filepath.Join(tmpDir, "cache")
	} else {
		// Convert relative paths to absolute paths
		if !filepath.IsAbs(workDir) {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			workDir = filepath.Join(wd, workDir)
		}
		if cacheDir != "" && !filepath.IsAbs(cacheDir) {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			cacheDir = filepath.Join(wd, cacheDir)
		}
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return err
	}

	reposToCreate := env.Repositories
	if withRepoEntrypoint {
		// Create all repos in the cache
		for _, repo := range reposToCreate {
			repoName := fmt.Sprintf("%s-%s", env.Name, repo.Name)
			repoPath := filepath.Join(cacheDir, "repos", owner, repoName, repo.Branch)
			if err := os.MkdirAll(repoPath, 0755); err != nil {
				return err
			}
			if err := createRepoFiles(repoPath, &repo, env.Name, owner); err != nil {
				return err
			}
		}
	} else {
		// In local mode, the first repo is the "workdir" repo, the rest are cached.
		workdirRepo := env.Repositories[0]
		cachedRepos := env.Repositories[1:]

		// Create the workdir repo
		repoName := fmt.Sprintf("%s-%s", env.Name, workdirRepo.Name)
		repoPath := filepath.Join(workDir, repoName)
		if err := os.MkdirAll(repoPath, 0755); err != nil {
			return err
		}
		if err := createRepoFiles(repoPath, &workdirRepo, env.Name, owner); err != nil {
			return err
		}

		// Create the cached repos
		for _, repo := range cachedRepos {
			repoName := fmt.Sprintf("%s-%s", env.Name, repo.Name)
			repoPath := filepath.Join(cacheDir, "repos", owner, repoName, repo.Branch)
			if err := os.MkdirAll(repoPath, 0755); err != nil {
				return err
			}
			if err := createRepoFiles(repoPath, &repo, env.Name, owner); err != nil {
				return err
			}
		}
	}

	output := SetupOutput{
		WorkDir:  workDir,
		CacheDir: cacheDir,
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
}

func setupRemote(cmd *cobra.Command, env *e2e.TestEnvironmentDef, owner string) error {
	withRepoEntrypoint, _ := cmd.Flags().GetBool("with-repo-entrypoint")
	client, err := e2e.GetClient()
	if err != nil {
		return err
	}

	for _, repoDef := range env.Repositories {
		repoName := fmt.Sprintf("%s-%s", env.Name, repoDef.Name)
		// Delete if it exists
		_, err := client.Repositories.Delete(context.Background(), owner, repoName)
		if err != nil {
			if _, ok := err.(*github.ErrorResponse); !ok || err.(*github.ErrorResponse).Response.StatusCode != 404 {
				return err
			}
		}

		// Create repo
		_, _, err = client.Repositories.Create(context.Background(), owner, &github.Repository{
			Name: &repoName,
		})
		if err != nil {
			return err
		}
		time.Sleep(2 * time.Second) // Give GitHub some time to create the repo

		// Create a dummy file to ensure the main branch is created
		_, _, err = client.Repositories.CreateFile(context.Background(), owner, repoName, "README.md", &github.RepositoryContentFileOptions{
			Message: github.String("initial commit"),
			Content: []byte("# " + repoName),
			Branch:  github.String("main"),
		})
		if err != nil {
			return err
		}

		// Create tako.yml
		takoConfig := buildTakoConfig(env.Name, owner, &repoDef)
		content, err := yaml.Marshal(takoConfig)
		if err != nil {
			return err
		}
		_, _, err = client.Repositories.CreateFile(context.Background(), owner, repoName, "tako.yml", &github.RepositoryContentFileOptions{
			Message: github.String("add tako.yml"),
			Content: content,
			Branch:  github.String("main"),
		})
		if err != nil {
			return err
		}

		// Create other files from templates
		for _, fileDef := range repoDef.Files {
			content, err := e2e.GetTemplate(fileDef.Template)
			if err != nil {
				return err
			}
			// Replace placeholders
			content = strings.ReplaceAll(content, "{{.Owner}}", owner)
			content = strings.ReplaceAll(content, "{{.EnvName}}", env.Name)

			_, _, err = client.Repositories.CreateFile(context.Background(), owner, repoName, fileDef.Path, &github.RepositoryContentFileOptions{
				Message: github.String("add " + fileDef.Path),
				Content: []byte(content),
				Branch:  github.String("main"),
			})
			if err != nil {
				return err
			}
		}
	}

	tmpDir, err := os.MkdirTemp("", "tako-e2e-")
	if err != nil {
		return err
	}

	// Clone the entrypoint repo for path mode
	if !withRepoEntrypoint {
		repoName := fmt.Sprintf("%s-%s", env.Name, env.Repositories[0].Name)
		cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repoName)
		if err := git.Clone(cloneURL, filepath.Join(tmpDir, repoName)); err != nil {
			return err
		}
	}

	output := SetupOutput{
		WorkDir:  tmpDir,
		CacheDir: filepath.Join(tmpDir, "cache"),
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
}

func createRepoFiles(repoPath string, repoDef *e2e.RepositoryDef, envName, owner string) error {
	// Init git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to init git repo in %s: %w", repoPath, err)
	}
	remoteURL := fmt.Sprintf("https://github.com/%s/%s-%s.git", owner, envName, repoDef.Name)
	cmd = exec.Command("git", "remote", "add", "origin", remoteURL)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add remote in %s: %w", repoPath, err)
	}

	// Create tako.yml
	takoConfig := buildTakoConfig(envName, owner, repoDef)
	content, err := yaml.Marshal(takoConfig)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(repoPath, "tako.yml"), content, 0644); err != nil {
		return err
	}

	// Create other files from templates
	for _, fileDef := range repoDef.Files {
		content, err := e2e.GetTemplate(fileDef.Template)
		if err != nil {
			return err
		}
		// Replace placeholders
		content = strings.ReplaceAll(content, "{{.Owner}}", owner)
		content = strings.ReplaceAll(content, "{{.EnvName}}", envName)
		filePath := filepath.Join(repoPath, fileDef.Path)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

func buildTakoConfig(envName, owner string, repoDef *e2e.RepositoryDef) *config.Config {
	takoConfig := &config.Config{
		Version: "0.1.0",
		Artifacts: map[string]config.Artifact{
			"default": {Path: ".", Ecosystem: "generic"},
		},
		Workflows: map[string]config.Workflow{
			"default": {
				Steps: []config.WorkflowStep{
					{Run: "echo 'default workflow'"},
				},
			},
		},
		Subscriptions: []config.Subscription{},
	}

	if envName == "simple-graph" {
		takoConfig.Workflows["test-workflow"] = config.Workflow{
			Inputs: map[string]config.WorkflowInput{
				"environment": {
					Type: "string",
					Validation: config.WorkflowInputValidation{
						Enum: []string{"dev", "staging", "prod"},
					},
				},
			},
			Steps: []config.WorkflowStep{
				{
					ID:  "validate_input",
					Run: "echo 'Deploying to {{ .inputs.environment }}'",
				},
				{
					ID:  "process_output",
					Run: "echo 'processed-{{ .steps.validate_input.outputs.result }}'",
					Produces: &config.WorkflowStepProduces{
						Outputs: map[string]string{
							"final_result": "from_stdout",
						},
					},
				},
			},
		}
	}

	for _, dep := range repoDef.Dependencies {
		takoConfig.Subscriptions = append(takoConfig.Subscriptions, config.Subscription{
			Artifact: fmt.Sprintf("%s/%s-%s:default", owner, envName, dep),
			Events:   []string{"updated"},
			Workflow: "default",
		})
	}
	return takoConfig
}
