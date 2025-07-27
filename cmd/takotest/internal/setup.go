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

		// Check if repo already exists before trying to delete/create
		repo, _, err := client.Repositories.Get(context.Background(), owner, repoName)
		repoExists := (err == nil && repo != nil)

		if repoExists {
			// Repository exists, check if it has the right structure
			_, _, _, err = client.Repositories.GetContents(context.Background(), owner, repoName, "tako.yml", nil)
			if err == nil {
				// Repository already has tako.yml, reuse it
				fmt.Printf("Repository %s already exists and is properly configured, reusing it\n", repoName)
				continue
			}

			// Repository exists but doesn't have proper structure, we need to recreate files
			fmt.Printf("Repository %s exists but needs file updates\n", repoName)
		} else {
			// Repository doesn't exist, we need to create it
			fmt.Printf("Creating repository %s\n", repoName)

			// Add exponential backoff for rate limiting
			retryDelay := 30 * time.Second // Start with longer delay for secondary rate limits
			maxRetries := 3                // Reduce retries to avoid prolonged failures

			// Create repo with retry logic (skip deletion since repo doesn't exist)
			for attempt := 0; attempt < maxRetries; attempt++ {
				_, _, err = client.Repositories.Create(context.Background(), owner, &github.Repository{
					Name: &repoName,
				})
				if err != nil {
					if errResp, ok := err.(*github.ErrorResponse); ok && errResp.Response.StatusCode == 403 && strings.Contains(errResp.Message, "rate limit") {
						waitTime := retryDelay * time.Duration(attempt+1)
						fmt.Printf("Rate limit hit during repo creation, waiting %v before retry (attempt %d/%d)\n", waitTime, attempt+1, maxRetries)
						time.Sleep(waitTime)
						continue
					}
					return fmt.Errorf("failed to create repository %s: %w", repoName, err)
				}
				break
			}

			if err != nil {
				return fmt.Errorf("failed to create repository %s after %d attempts: %w", repoName, maxRetries, err)
			}

			time.Sleep(8 * time.Second) // Give GitHub more time to create the repo
		}

		// Helper function to create or update file with retry logic
		createOrUpdateFileWithRetry := func(path string, content []byte, message string) error {
			maxFileRetries := 3
			for attempt := 0; attempt < maxFileRetries; attempt++ {
				// First try to create the file
				_, _, err := client.Repositories.CreateFile(context.Background(), owner, repoName, path, &github.RepositoryContentFileOptions{
					Message: github.String(message),
					Content: content,
					Branch:  github.String("main"),
				})

				if err != nil {
					// If file already exists, try to update it instead
					if errResp, ok := err.(*github.ErrorResponse); ok && errResp.Response.StatusCode == 422 {
						// File already exists, get its SHA and update it
						fileContent, _, _, err := client.Repositories.GetContents(context.Background(), owner, repoName, path, nil)
						if err != nil {
							return fmt.Errorf("failed to get existing file %s: %w", path, err)
						}

						_, _, err = client.Repositories.UpdateFile(context.Background(), owner, repoName, path, &github.RepositoryContentFileOptions{
							Message: github.String(message + " (update)"),
							Content: content,
							Branch:  github.String("main"),
							SHA:     fileContent.SHA,
						})
						if err != nil {
							if errResp, ok := err.(*github.ErrorResponse); ok && errResp.Response.StatusCode == 403 && strings.Contains(errResp.Message, "rate limit") {
								waitTime := time.Duration(attempt+1) * 5 * time.Second
								fmt.Printf("Rate limit hit during file update (%s), waiting %v before retry (attempt %d/%d)\n", path, waitTime, attempt+1, maxFileRetries)
								time.Sleep(waitTime)
								continue
							}
							return fmt.Errorf("failed to update file %s: %w", path, err)
						}
					} else if errResp, ok := err.(*github.ErrorResponse); ok && errResp.Response.StatusCode == 403 && strings.Contains(errResp.Message, "rate limit") {
						waitTime := time.Duration(attempt+1) * 5 * time.Second
						fmt.Printf("Rate limit hit during file creation (%s), waiting %v before retry (attempt %d/%d)\n", path, waitTime, attempt+1, maxFileRetries)
						time.Sleep(waitTime)
						continue
					} else {
						return fmt.Errorf("failed to create file %s: %w", path, err)
					}
				}
				// Add small delay between file operations
				time.Sleep(2 * time.Second)
				return nil
			}
			return fmt.Errorf("failed to create/update file %s after %d attempts", path, maxFileRetries)
		}

		// Create a dummy file to ensure the main branch is created (only if repo was just created)
		if !repoExists {
			err = createOrUpdateFileWithRetry("README.md", []byte("# "+repoName), "initial commit")
			if err != nil {
				return err
			}
		}

		// Create tako.yml
		takoConfig := buildTakoConfig(env.Name, owner, &repoDef)
		content, err := yaml.Marshal(takoConfig)
		if err != nil {
			return err
		}
		err = createOrUpdateFileWithRetry("tako.yml", content, "add tako.yml")
		if err != nil {
			return err
		}

		// Create other files from templates
		for _, fileDef := range repoDef.Files {
			templateContent, err := e2e.GetTemplate(fileDef.Template)
			if err != nil {
				return err
			}
			// Replace placeholders
			templateContent = strings.ReplaceAll(templateContent, "{{.Owner}}", owner)
			templateContent = strings.ReplaceAll(templateContent, "{{.EnvName}}", env.Name)

			err = createOrUpdateFileWithRetry(fileDef.Path, []byte(templateContent), "add "+fileDef.Path)
			if err != nil {
				return err
			}
		}
	}

	tmpDir, err := os.MkdirTemp("", "tako-e2e-")
	if err != nil {
		return err
	}

	// Clone repositories based on entrypoint mode
	if !withRepoEntrypoint {
		// Clone entrypoint repo for path mode
		repoName := fmt.Sprintf("%s-%s", env.Name, env.Repositories[0].Name)
		cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repoName)
		if err := git.Clone(cloneURL, filepath.Join(tmpDir, repoName)); err != nil {
			return err
		}
	} else {
		// Clone all repos to cache for repo entrypoint mode
		cacheDir := filepath.Join(tmpDir, "cache")
		for _, repoDef := range env.Repositories {
			repoName := fmt.Sprintf("%s-%s", env.Name, repoDef.Name)
			cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repoName)
			repoPath := filepath.Join(cacheDir, "repos", owner, repoName, repoDef.Branch)
			if err := os.MkdirAll(repoPath, 0755); err != nil {
				return err
			}
			if err := git.Clone(cloneURL, repoPath); err != nil {
				return err
			}
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

	// Create tako.yml (special handling for malformed-config environment)
	if envName == "malformed-config" {
		// For malformed config, use the template file instead of generating config
		malformedContent, err := e2e.GetTemplate("malformed-config/malformed-tako.yml")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(repoPath, "tako.yml"), []byte(malformedContent), 0644); err != nil {
			return err
		}
	} else {
		// Normal config generation
		takoConfig := buildTakoConfig(envName, owner, repoDef)
		content, err := yaml.Marshal(takoConfig)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(repoPath, "tako.yml"), content, 0644); err != nil {
			return err
		}
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
					Run: "echo 'Deploying to {{ .Inputs.environment }}'",
				},
				{
					ID:  "process_output",
					Run: "echo 'processed-{{ .Steps.validate_input.result }}'",
					Produces: &config.WorkflowStepProduces{
						Outputs: map[string]string{
							"final_result": "from_stdout",
						},
					},
				},
			},
		}
	}

	if envName == "single-repo-workflow" {
		// advanced-input-workflow: Tests input validation, enum constraints, and template security functions
		takoConfig.Workflows["advanced-input-workflow"] = config.Workflow{
			Inputs: map[string]config.WorkflowInput{
				"environment": {
					Type:     "string",
					Required: true,
					Validation: config.WorkflowInputValidation{
						Enum: []string{"dev", "staging", "prod"},
					},
				},
				"version": {
					Type:    "string",
					Default: "1.0.0",
				},
				"replicas": {
					Type:    "string",
					Default: "3",
				},
				"debug": {
					Type:    "string",
					Default: "false",
				},
			},
			Steps: []config.WorkflowStep{
				{
					ID:  "validate_inputs",
					Run: "echo 'Environment: {{ .Inputs.environment }}, Version: {{ .Inputs.version }}, Replicas: {{ .Inputs.replicas }}, Debug: {{ .Inputs.debug }}'",
				},
				{
					ID:  "process_with_templates",
					Run: "echo 'Processed: {{ .Steps.validate_inputs.result | shell_quote }}' && echo 'JSON: {{ .Inputs.environment | json_escape }}' && echo 'URL: {{ .Inputs.version | url_encode }}'",
					Produces: &config.WorkflowStepProduces{
						Outputs: map[string]string{
							"processed_data": "from_stdout",
						},
					},
				},
				{
					ID:  "final_step",
					Run: "echo 'Final result: {{ .Steps.process_with_templates.outputs.processed_data }}'",
					Produces: &config.WorkflowStepProduces{
						Outputs: map[string]string{
							"final_output": "from_stdout",
						},
					},
				},
			},
		}

		// step-output-workflow: Tests step output capture and passing between sequential steps
		takoConfig.Workflows["step-output-workflow"] = config.Workflow{
			Steps: []config.WorkflowStep{
				{
					ID:  "step1",
					Run: "echo 'output1'",
					Produces: &config.WorkflowStepProduces{
						Outputs: map[string]string{
							"result": "from_stdout",
						},
					},
				},
				{
					ID:  "step2",
					Run: "echo 'Step1 output was: {{ .Steps.step1.outputs.result }}'",
					Produces: &config.WorkflowStepProduces{
						Outputs: map[string]string{
							"combined": "from_stdout",
						},
					},
				},
				{
					ID:  "step3",
					Run: "echo 'Final: {{ .Steps.step2.outputs.combined }}'",
				},
			},
		}

		// error-handling-workflow: Tests workflow failure scenarios and step execution halting
		takoConfig.Workflows["error-handling-workflow"] = config.Workflow{
			Steps: []config.WorkflowStep{
				{
					ID:  "success_step",
					Run: "echo 'This step succeeds'",
				},
				{
					ID:  "failure_step",
					Run: "echo 'This step will fail' && exit 1",
				},
				{
					ID:  "should_not_run",
					Run: "echo 'This should not execute'",
				},
			},
		}

		// template-variable-workflow: Tests template variable resolution with default/custom values and security functions
		takoConfig.Workflows["template-variable-workflow"] = config.Workflow{
			Inputs: map[string]config.WorkflowInput{
				"message": {
					Type:    "string",
					Default: "Hello World",
				},
				"count": {
					Type:    "string",
					Default: "5",
				},
			},
			Steps: []config.WorkflowStep{
				{
					ID:  "test_variables",
					Run: "echo 'Message: {{ .Inputs.message | shell_quote }}' && echo 'Count: {{ .Inputs.count }}' && echo 'Combined: {{ .Inputs.message | shell_quote }}-{{ .Inputs.count }}'",
					Produces: &config.WorkflowStepProduces{
						Outputs: map[string]string{
							"message_output": "from_stdout",
						},
					},
				},
				{
					ID:  "test_security_functions",
					Run: "echo 'Shell quoted: {{ .Inputs.message | shell_quote }}' && echo 'JSON escaped: {{ .Inputs.message | json_escape }}' && echo 'HTML escaped: {{ .Inputs.message | html_escape }}'",
				},
			},
		}

		// long-running-workflow: Tests multi-step execution with file operations (foundation for future resume testing)
		takoConfig.Workflows["long-running-workflow"] = config.Workflow{
			Steps: []config.WorkflowStep{
				{
					ID:  "prepare",
					Run: "chmod +x ./scripts/prepare.sh && ./scripts/prepare.sh",
					Produces: &config.WorkflowStepProduces{
						Outputs: map[string]string{
							"prepare_result": "from_stdout",
						},
					},
				},
				{
					ID:  "long_process",
					Run: "chmod +x ./scripts/long-process.sh && ./scripts/long-process.sh",
					Produces: &config.WorkflowStepProduces{
						Outputs: map[string]string{
							"process_result": "from_stdout",
						},
					},
				},
				{
					ID:  "finalize",
					Run: "chmod +x ./scripts/finalize.sh && ./scripts/finalize.sh",
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
