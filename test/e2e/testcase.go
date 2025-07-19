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

type TestCase struct {
	Name               string
	Dirty              bool
	WithRepoEntryPoint bool
	Repositories       []Repository
	ExpectedError      string
}

type Repository struct {
	Owner      string
	Name       string
	TakoConfig *config.TakoConfig
	CloneURL   string
}

func GetTestCases(owner string) map[string]TestCase {
	return map[string]TestCase{
		"simple-graph": {
			Name:  "simple-graph",
			Dirty: false,
			Repositories: []Repository{
				{
					Owner: owner,
					Name:  "repo-a",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-a",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-b:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-b",
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
		"simple-graph-with-repo-flag": {
			Name:               "simple-graph-with-repo-flag",
			WithRepoEntryPoint: true,
			Dirty:              false,
			Repositories: []Repository{
				{
					Owner: owner,
					Name:  "repo-a",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-a",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-b:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-b",
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
			Name:  "complex-graph",
			Dirty: false,
			Repositories: []Repository{
				{
					Owner: owner,
					Name:  "repo-a",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-a",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-b:main", owner)},
							{Repo: fmt.Sprintf("%s/repo-d:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-b",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-b",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-c:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-c",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-c",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-e:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-d",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-d",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-e:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-e",
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
			Name:  "deep-graph",
			Dirty: false,
			Repositories: []Repository{
				{
					Owner: owner,
					Name:  "repo-x",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-x",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-y:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-y",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-y",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-z:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-z",
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
			Name:  "diamond-dependency-graph",
			Dirty: false,
			Repositories: []Repository{
				{
					Owner: owner,
					Name:  "repo-a",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-a",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-b:main", owner)},
							{Repo: fmt.Sprintf("%s/repo-d:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-b",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-b",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-c:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-c",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-c",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-e:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-d",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-d",
						},
						Dependents: []config.Dependent{
							{Repo: fmt.Sprintf("%s/repo-e:main", owner)},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-e",
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
		"circular-dependency-graph": {
			Name:  "circular-dependency-graph",
			Dirty: false,
			Repositories: []Repository{
				{
					Owner: owner,
					Name:  "repo-circ-a",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-circ-a",
						},
						Dependents: []config.Dependent{
							{Repo: "../repo-circ-b:main"},
						},
					},
				},
				{
					Owner: owner,
					Name:  "repo-circ-b",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-circ-b",
						},
						Dependents: []config.Dependent{
							{Repo: "../repo-circ-a:main"},
						},
					},
				},
			},
			ExpectedError: "circular dependency detected: repo-circ-a -> repo-circ-b -> repo-circ-a",
		},
	}
}

var TestCases = GetTestCases(Org)

func GetClient() (*github.Client, error) {
	token := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_PERSONAL_ACCESS_TOKEN is not set")
	}
	return github.NewClient(nil).WithAuthToken(token), nil
}

func (tc *TestCase) Setup(client *github.Client) error {
	if !tc.Dirty {
		// Check if all repositories exist
		allExist := true
		for i := range tc.Repositories {
			repo := &tc.Repositories[i]
			ghRepo, _, err := client.Repositories.Get(context.Background(), repo.Owner, repo.Name)
			if err != nil {
				allExist = false
				break
			}
			repo.CloneURL = *ghRepo.CloneURL
		}
		if allExist {
			return nil
		}
	}

	for i := range tc.Repositories {
		repo := &tc.Repositories[i]

		// Check if the repo exists, and if so, delete it
		_, _, err := client.Repositories.Get(context.Background(), repo.Owner, repo.Name)
		if err == nil {
			_, err = client.Repositories.Delete(context.Background(), repo.Owner, repo.Name)
			if err != nil {
				return err
			}
		}

		createdRepo, _, err := client.Repositories.Create(context.Background(), repo.Owner, &github.Repository{
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

		_, _, err = client.Repositories.CreateFile(context.Background(), repo.Owner, repo.Name, "tako.yml", &github.RepositoryContentFileOptions{
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
	tmpDir := filepath.Join(os.TempDir(), tc.Name)
	if err := os.RemoveAll(tmpDir); err != nil {
		return "", err
	}

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", err
	}

	cacheDir := filepath.Join(tmpDir, "cache")
	workDir := filepath.Join(tmpDir, "workdir")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", err
	}

	reposToCreateInCache := tc.Repositories
	if !tc.WithRepoEntryPoint {
		if tc.Name == "circular-dependency-graph" {
			reposToCreateInCache = []Repository{}
		} else if len(tc.Repositories) > 1 {
			reposToCreateInCache = tc.Repositories[1:]
		} else {
			reposToCreateInCache = []Repository{}
		}
	}

	for _, repo := range reposToCreateInCache {
		repoPath := filepath.Join(cacheDir, "repos", repo.Owner, repo.Name)
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

	if !tc.WithRepoEntryPoint {
		reposToCreateInWorkdir := []Repository{tc.Repositories[0]}
		if tc.Name == "circular-dependency-graph" {
			reposToCreateInWorkdir = tc.Repositories
		}
		for _, repo := range reposToCreateInWorkdir {
			repoPath := filepath.Join(workDir, repo.Name)
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
	}

	return tmpDir, nil
}

func (tc *TestCase) Cleanup(client *github.Client) error {
	if !tc.Dirty {
		return nil
	}
	for _, repo := range tc.Repositories {
		_, err := client.Repositories.Delete(context.Background(), repo.Owner, repo.Name)
		if err != nil {
			return err
		}
	}
	return nil
}
