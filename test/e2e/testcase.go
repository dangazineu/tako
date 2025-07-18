package e2e

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"time"

	"github.com/dangazineu/tako/internal/config"
	"github.com/google/go-github/v63/github"
)

const (
	Org = "tako-test"
)

type TestCase struct {
	Name         string
	Repositories []Repository
}

type Repository struct {
	Name       string
	TakoConfig *config.TakoConfig
	CloneURL   string
}

var TestCases = map[string]TestCase{
	"simple-graph": {
		Name: "simple-graph",
		Repositories: []Repository{
			{
				Name: "repo-a",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-a",
					},
					Dependents: []config.Dependent{
						{Repo: "tako-test/repo-b:main"},
					},
				},
			},
			{
				Name: "repo-b",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-b",
					},
					Dependents: []config.Dependent{},
				},
			},
		},
	},
	"complex-graph": {
		Name: "complex-graph",
		Repositories: []Repository{
			{
				Name: "repo-a",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-a",
					},
					Dependents: []config.Dependent{
						{Repo: "tako-test/repo-b:main"},
						{Repo: "tako-test/repo-d:main"},
					},
				},
			},
			{
				Name: "repo-b",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-b",
					},
					Dependents: []config.Dependent{
						{Repo: "tako-test/repo-c:main"},
					},
				},
			},
			{
				Name: "repo-c",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-c",
					},
					Dependents: []config.Dependent{
						{Repo: "tako-test/repo-e:main"},
					},
				},
			},
			{
				Name: "repo-d",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-d",
					},
					Dependents: []config.Dependent{
						{Repo: "tako-test/repo-e:main"},
					},
				},
			},
			{
				Name: "repo-e",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-e",
					},
					Dependents: []config.Dependent{},
				},
			},
		},
	},
	"deep-graph": {
		Name: "deep-graph",
		Repositories: []Repository{
			{
				Name: "repo-x",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-x",
					},
					Dependents: []config.Dependent{
						{Repo: "tako-test/repo-y:main"},
					},
				},
			},
			{
				Name: "repo-y",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-y",
					},
					Dependents: []config.Dependent{
						{Repo: "tako-test/repo-z:main"},
					},
				},
			},
			{
				Name: "repo-z",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-z",
					},
					Dependents: []config.Dependent{},
				},
			},
		},
	},
	"diamond-dependency-graph": {
		Name: "diamond-dependency-graph",
		Repositories: []Repository{
			{
				Name: "repo-a",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-a",
					},
					Dependents: []config.Dependent{
						{Repo: "tako-test/repo-b:main"},
						{Repo: "tako-test/repo-d:main"},
					},
				},
			},
			{
				Name: "repo-b",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-b",
					},
					Dependents: []config.Dependent{
						{Repo: "tako-test/repo-c:main"},
					},
				},
			},
			{
				Name: "repo-c",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-c",
					},
					Dependents: []config.Dependent{
						{Repo: "tako-test/repo-e:main"},
					},
				},
			},
			{
				Name: "repo-d",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-d",
					},
					Dependents: []config.Dependent{
						{Repo: "tako-test/repo-e:main"},
					},
				},
			},
			{
				Name: "repo-e",
				TakoConfig: &config.TakoConfig{
					Version: "0.1.0",
					Metadata: config.Metadata{
						Name: "repo-e",
					},
					Dependents: []config.Dependent{},
				},
			},
		},
	},
}

func GetClient() (*github.Client, error) {
	token := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_PERSONAL_ACCESS_TOKEN is not set")
	}
	return github.NewClient(nil).WithAuthToken(token), nil
}

func (tc *TestCase) Setup(client *github.Client) error {
	for i := range tc.Repositories {
		repo := &tc.Repositories[i]

		// Check if the repo exists, and if so, delete it
		_, _, err := client.Repositories.Get(context.Background(), Org, repo.Name)
		if err == nil {
			_, err = client.Repositories.Delete(context.Background(), Org, repo.Name)
			if err != nil {
				return err
			}
		}

		createdRepo, _, err := client.Repositories.Create(context.Background(), Org, &github.Repository{
			Name: &repo.Name,
		})
		if err != nil {
			return err
		}
		repo.CloneURL = *createdRepo.CloneURL

		content, err := yaml.Marshal(repo.TakoConfig)
		if err != nil {
			return err
		}

		_, _, err = client.Repositories.CreateFile(context.Background(), Org, repo.Name, "tako.yml", &github.RepositoryContentFileOptions{
			Message: github.String("initial commit"),
			Content: content,
			Branch:  github.String("main"),
		})
		if err != nil {
			return err
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

func (tc *TestCase) SetupLocal() (string, error) {
	tmpDir, err := os.MkdirTemp("", tc.Name)
	if err != nil {
		return "", err
	}

	for _, repo := range tc.Repositories {
		repoPath := filepath.Join(tmpDir, Org, repo.Name)
		if err := os.MkdirAll(repoPath, 0755); err != nil {
			return "", err
		}
		filePath := filepath.Join(repoPath, "tako.yml")
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return "", err
		}

		content, err := yaml.Marshal(repo.TakoConfig)
		if err != nil {
			return "", err
		}

		err = os.WriteFile(filePath, content, 0644)
		if err != nil {
			return "", err
		}
	}
	return tmpDir, nil
}

func (tc *TestCase) Cleanup(client *github.Client) error {
	for _, repo := range tc.Repositories {
		_, err := client.Repositories.Delete(context.Background(), Org, repo.Name)
		if err != nil {
			return err
		}
	}
	return nil
}
