package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dangazineu/tako/internal/config"
	"github.com/google/go-github/v63/github"
	"sigs.k8s.io/yaml"
)


type TestCase struct {
	Name         string
	Dirty        bool
	Repositories []Repository
}

type Repository struct {
	Owner      string
	Name       string
	TakoConfig *config.TakoConfig
	CloneURL   string
}

func GetTestCases(owner string) map[string]TestCase {
	testCases := map[string]TestCase{
		"simple-graph": {
			Name:  "simple-graph",
			Dirty: false,
			Repositories: []Repository{
				{
					Name: "repo-a",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-a",
						},
						Dependents: []config.Dependent{
							{Repo: "repo-b:main"},
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
			Name:  "complex-graph",
			Dirty: false,
			Repositories: []Repository{
				{
					Name: "repo-a",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-a",
						},
						Dependents: []config.Dependent{
							{Repo: "repo-b:main"},
							{Repo: "repo-d:main"},
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
							{Repo: "repo-c:main"},
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
							{Repo: "repo-e:main"},
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
							{Repo: "repo-e:main"},
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
			Name:  "deep-graph",
			Dirty: false,
			Repositories: []Repository{
				{
					Name: "repo-x",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-x",
						},
						Dependents: []config.Dependent{
							{Repo: "repo-y:main"},
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
							{Repo: "repo-z:main"},
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
			Name:  "diamond-dependency-graph",
			Dirty: false,
			Repositories: []Repository{
				{
					Name: "repo-a",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-a",
						},
						Dependents: []config.Dependent{
							{Repo: "repo-b:main"},
							{Repo: "repo-d:main"},
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
							{Repo: "repo-c:main"},
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
							{Repo: "repo-e:main"},
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
							{Repo: "repo-e:main"},
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
		"circular-dependency-graph": {
			Name:  "circular-dependency-graph",
			Dirty: false,
			Repositories: []Repository{
				{
					Name: "repo-circ-a",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-circ-a",
						},
						Dependents: []config.Dependent{
							{Repo: "repo-circ-b:main"},
						},
					},
				},
				{
					Name: "repo-circ-b",
					TakoConfig: &config.TakoConfig{
						Version: "0.1.0",
						Metadata: config.Metadata{
							Name: "repo-circ-b",
						},
						Dependents: []config.Dependent{
							{Repo: "repo-circ-a:main"},
						},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		for i := range tc.Repositories {
			repo := &tc.Repositories[i]
			repo.Owner = owner
			repo.Name = fmt.Sprintf("%s-%s", name, repo.Name)
			repo.TakoConfig.Metadata.Name = repo.Name
			for j := range repo.TakoConfig.Dependents {
				dependent := &repo.TakoConfig.Dependents[j]
				repoAndRef := strings.Split(dependent.Repo, ":")
				depRepoName := repoAndRef[0]
				ref := ""
				if len(repoAndRef) > 1 {
					ref = ":" + repoAndRef[1]
				}
				dependent.Repo = fmt.Sprintf("%s/%s-%s%s", repo.Owner, name, depRepoName, ref)
			}
		}
	}
	return testCases
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
	if !tc.Dirty {
		// Check if the directory exists
		if _, err := os.Stat(tmpDir); err == nil {
			return tmpDir, nil
		}
	}

	if err := os.RemoveAll(tmpDir); err != nil {
		return "", err
	}

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", err
	}

	for _, repo := range tc.Repositories {
		repoPath := filepath.Join(tmpDir, repo.Owner, repo.Name)
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

func (tc *TestCase) GetRepoEntryPoint() string {
	return fmt.Sprintf("%s/%s:main", tc.Repositories[0].Owner, tc.Repositories[0].Name)
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
