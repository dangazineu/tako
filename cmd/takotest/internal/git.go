package internal

import (
	"context"
	"fmt"
	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/test/e2e"
	"github.com/google/go-github/v63/github"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func createLocalGitRepo(repoPath string, repoDef *e2e.RepositoryDef, envName, owner string, local bool) error {
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return err
	}

	// Create tako.yml
	takoConfig := &config.TakoConfig{
		Version: "0.1.0",
		Metadata: config.Metadata{
			Name: fmt.Sprintf("%s-%s", envName, repoDef.Name),
		},
	}
	for _, dep := range repoDef.Dependencies {
		depOwner := owner
		if local {
			depOwner = "local"
		}
		takoConfig.Dependents = append(takoConfig.Dependents, config.Dependent{Repo: fmt.Sprintf("%s/%s-%s:%s", depOwner, envName, dep, repoDef.Branch)})
	}
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
		content = strings.ReplaceAll(content, "{{.EnvName}}", repoDef.Name)
		if err := os.WriteFile(filepath.Join(repoPath, fileDef.Path), []byte(content), 0644); err != nil {
			return err
		}
	}

	// Git operations
	if err := runGitCmd(repoPath, "init"); err != nil {
		return err
	}
	if err := runGitCmd(repoPath, "add", "."); err != nil {
		return err
	}
	if err := runGitCmd(repoPath, "commit", "-m", "initial commit"); err != nil {
		return err
	}
	if repoDef.Branch != "main" {
		if err := runGitCmd(repoPath, "branch", "-M", repoDef.Branch); err != nil {
			return err
		}
	}

	return nil
}

func createRemoteAndPush(repoPath, owner, repoName string) error {
	client, err := e2e.GetClient()
	if err != nil {
		return err
	}

	// Delete if it exists
	_, err = client.Repositories.Delete(context.Background(), owner, repoName)
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
	time.Sleep(2 * time.Second) // Give GitHub some time

	remoteURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repoName)
	if err := runGitCmd(repoPath, "remote", "add", "origin", remoteURL); err != nil {
		return err
	}
	if err := runGitCmd(repoPath, "push", "-u", "origin", "HEAD"); err != nil {
		return err
	}
	return nil
}

func runGitCmd(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git command failed: %s\nOutput:\n%s", err, string(output))
	}
	return nil
}
