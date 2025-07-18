package graph

import (
	"bytes"
	"errors"
	"github.com/dangazineu/tako/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"testing"
)

func TestPrintGraph(t *testing.T) {
	root := &Node{
		Name: "root",
		Children: []*Node{
			{
				Name: "child1",
				Children: []*Node{
					{Name: "grandchild1"},
				},
			},
			{Name: "child2"},
		},
	}

	var buf bytes.Buffer
	PrintGraph(&buf, root)

	expected := "root\n├── child1\n│   └── grandchild1\n└── child2\n"
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestBuildGraph(t *testing.T) {
	t.Run("local", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoA := createRepo(t, tmpDir, "repo-a", &config.TakoConfig{
			Version: "0.1.0",
			Metadata: config.Metadata{
				Name: "repo-a",
			},
			Dependents: []config.Dependent{
				{Repo: "test/repo-b:main"},
			},
		})
		createRepo(t, filepath.Join(tmpDir, "repos", "test"), "repo-b", &config.TakoConfig{
			Version: "0.1.0",
			Metadata: config.Metadata{
				Name: "repo-b",
			},
			Dependents: []config.Dependent{},
		})

		root, err := BuildGraph(repoA, tmpDir, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if root.Name != "repo-a" {
			t.Errorf("expected root name to be 'repo-a', got %q", root.Name)
		}
		if len(root.Children) != 1 {
			t.Fatalf("expected 1 child, got %d", len(root.Children))
		}
		if root.Children[0].Name != "repo-b" {
			t.Errorf("expected child name to be 'repo-b', got %q", root.Children[0].Name)
		}
	})

	t.Run("remote", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := t.TempDir()

		repoA := createRepo(t, tmpDir, "repo-a", &config.TakoConfig{
			Version: "0.1.0",
			Metadata: config.Metadata{
				Name: "repo-a",
			},
			Dependents: []config.Dependent{
				{Repo: "test/repo-b:main"},
			},
		})
		originDir := t.TempDir()
		cmd := exec.Command("git", "init", "--bare")
		cmd.Dir = originDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to git init bare: %v", err)
		}
		cloneDir := filepath.Join(cacheDir, "repos", "test", "repo-b")
		if err := os.MkdirAll(filepath.Dir(cloneDir), 0755); err != nil {
			t.Fatalf("failed to create repo dir: %v", err)
		}
		cmd = exec.Command("git", "clone", originDir, cloneDir)
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to git clone: %v", err)
		}
		createRepo(t, cloneDir, "", &config.TakoConfig{
			Version: "0.1.0",
			Metadata: config.Metadata{
				Name: "repo-b",
			},
			Dependents: []config.Dependent{},
		})
		cmd = exec.Command("git", "push", "origin", "main")
		cmd.Dir = cloneDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to git push: %v", err)
		}

		root, err := BuildGraph(repoA, cacheDir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if root.Name != "repo-a" {
			t.Errorf("expected root name to be 'repo-a', got %q", root.Name)
		}
		if len(root.Children) != 1 {
			t.Fatalf("expected 1 child, got %d", len(root.Children))
		}
		if root.Children[0].Name != "repo-b" {
			t.Errorf("expected child name to be 'repo-b', got %q", root.Children[0].Name)
		}
	})

	t.Run("remote-not-cached", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := t.TempDir()

		repoA := createRepo(t, tmpDir, "repo-a", &config.TakoConfig{
			Version: "0.1.0",
			Metadata: config.Metadata{
				Name: "repo-a",
			},
			Dependents: []config.Dependent{
				{Repo: "test/repo-b:main"},
			},
		})

		originalClone := Clone
		defer func() { Clone = originalClone }()
		Clone = func(url, path string) error {
			originDir := t.TempDir()
			cmd := exec.Command("git", "init", "--bare")
			cmd.Dir = originDir
			if err := cmd.Run(); err != nil {
				t.Fatalf("failed to git init bare: %v", err)
			}
			cmd = exec.Command("git", "clone", originDir, path)
			if err := cmd.Run(); err != nil {
				t.Fatalf("failed to git clone: %v", err)
			}
			createRepo(t, path, "", &config.TakoConfig{
				Version: "0.1.0",
				Metadata: config.Metadata{
					Name: "repo-b",
				},
				Dependents: []config.Dependent{},
			})
			cmd = exec.Command("git", "push", "origin", "main")
			cmd.Dir = path
			if err := cmd.Run(); err != nil {
				t.Fatalf("failed to git push: %v", err)
			}
			return nil
		}

		root, err := BuildGraph(repoA, cacheDir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if root.Name != "repo-a" {
			t.Errorf("expected root name to be 'repo-a', got %q", root.Name)
		}
		if len(root.Children) != 1 {
			t.Fatalf("expected 1 child, got %d", len(root.Children))
		}
		if root.Children[0].Name != "repo-b" {
			t.Errorf("expected child name to be 'repo-b', got %q", root.Children[0].Name)
		}
	})

	t.Run("circular-dependency", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoA := createRepo(t, tmpDir, "repo-circ-a", &config.TakoConfig{
			Version: "0.1.0",
			Metadata: config.Metadata{
				Name: "repo-circ-a",
			},
			Dependents: []config.Dependent{
				{Repo: "test/repo-circ-b:main"},
			},
		})
		createRepo(t, filepath.Join(tmpDir, "repos", "test"), "repo-circ-b", &config.TakoConfig{
			Version: "0.1.0",
			Metadata: config.Metadata{
				Name: "repo-circ-b",
			},
			Dependents: []config.Dependent{
				{Repo: "test/repo-circ-a:main"},
			},
		})
		createRepo(t, filepath.Join(tmpDir, "repos", "test"), "repo-circ-a", &config.TakoConfig{
			Version: "0.1.0",
			Metadata: config.Metadata{
				Name: "repo-circ-a",
			},
			Dependents: []config.Dependent{
				{Repo: "test/repo-circ-b:main"},
			},
		})

		_, err := BuildGraph(repoA, tmpDir, true)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		var circularDependencyError *CircularDependencyError
		if !errors.As(err, &circularDependencyError) {
			t.Fatalf("expected a circular dependency error, got %T", err)
		}
	})
}

func createRepo(t *testing.T, dir, name string, cfg *config.TakoConfig) string {
	repoDir := filepath.Join(dir, name)
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git init: %v", err)
	}
	cmd = exec.Command("git", "config", "user.email", "test@tako.dev")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to set git user.email: %v", err)
	}
	cmd = exec.Command("git", "config", "user.name", "Tako Test")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to set git user.name: %v", err)
	}
	// create tako.yml
	takoFile := filepath.Join(repoDir, "tako.yml")
	// marshal config to yaml
	content, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(takoFile, content, 0644); err != nil {
		t.Fatalf("failed to write tako.yml: %v", err)
	}
	cmd = exec.Command("git", "add", "tako.yml")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}
	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}
	return repoDir
}
