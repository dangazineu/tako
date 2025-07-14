package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	Name     string
	Files    map[string]string
	CloneURL string
}

var TestCases = map[string]TestCase{
	"simple-graph": {
		Name: "simple-graph",
		Repositories: []Repository{
			{
				Name: "repo-a",
				Files: map[string]string{
					"tako.yml": `
version: 0.1.0
metadata:
  name: repo-a
dependents:
  - repo: ../repo-b:main
`,
				},
			},
			{
				Name: "repo-b",
				Files: map[string]string{
					"tako.yml": `
version: 0.1.0
metadata:
  name: repo-b
dependents: []
`,
				},
			},
		},
	},
	"complex-graph": {
		Name: "complex-graph",
		Repositories: []Repository{
			{
				Name: "repo-a",
				Files: map[string]string{
					"tako.yml": `
version: 0.1.0
metadata:
  name: repo-a
dependents:
  - repo: ../repo-b:main
  - repo: ../repo-d:main
`,
				},
			},
			{
				Name: "repo-b",
				Files: map[string]string{
					"tako.yml": `
version: 0.1.0
metadata:
  name: repo-b
dependents:
  - repo: ../repo-c:main
`,
				},
			},
			{
				Name: "repo-c",
				Files: map[string]string{
					"tako.yml": `
version: 0.1.0
metadata:
  name: repo-c
dependents:
  - repo: ../repo-e:main
`,
				},
			},
			{
				Name: "repo-d",
				Files: map[string]string{
					"tako.yml": `
version: 0.1.0
metadata:
  name: repo-d
dependents:
  - repo: ../repo-e:main
`,
				},
			},
			{
				Name: "repo-e",
				Files: map[string]string{
					"tako.yml": `
version: 0.1.0
metadata:
  name: repo-e
dependents: []
`,
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

		for path, content := range repo.Files {
			_, _, err := client.Repositories.CreateFile(context.Background(), Org, repo.Name, path, &github.RepositoryContentFileOptions{
				Message: github.String("initial commit"),
				Content: []byte(content),
			})
			if err != nil {
				return err
			}
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

func (tc *TestCase) SetupLocal(baseDir string) error {
	for _, repo := range tc.Repositories {
		repoPath := filepath.Join(baseDir, repo.Name)
		os.MkdirAll(repoPath, 0755)
		for path, content := range repo.Files {
			filePath := filepath.Join(repoPath, path)
			os.MkdirAll(filepath.Dir(filePath), 0755)
			err := os.WriteFile(filePath, []byte(content), 0644)
			if err != nil {
				return err
			}
		}
	}
	return nil
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
