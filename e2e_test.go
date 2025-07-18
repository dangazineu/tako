//go:build e2e
// +build e2e

package main

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dangazineu/tako/test/e2e"
	"sigs.k8s.io/yaml"
)

var (
	local                 = flag.Bool("local", false, "run local tests")
	remote                = flag.Bool("remote", false, "run remote tests")
	withRepoEntryPoint    = flag.Bool("with-repo-entrypoint", false, "run tests with --repo flag")
	withoutRepoEntryPoint = flag.Bool("without-repo-entrypoint", false, "run tests with --root flag")
)

func findProjectRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func TestE2E(t *testing.T) {
	if !*local && !*remote {
		t.Fatal("either -local or -remote must be set")
	}
	if !*withRepoEntryPoint && !*withoutRepoEntryPoint {
		t.Fatal("either -with-repo-entrypoint or -without-repo-entrypoint must be set")
	}

	for name, tc := range e2e.GetTestCases(e2e.Org) {
		name, tc := name, tc // capture range variable
		t.Run(name, func(t *testing.T) {
			if *local {
				localTC := tc
				t.Run("local", func(t *testing.T) {
					if *withoutRepoEntryPoint {
						t.Run("without-repo-entrypoint", func(t *testing.T) {
							runTest(t, &localTC, "local", "path")
						})
					}
				})
			}
			if *remote {
				remoteTC := tc
				t.Run("remote", func(t *testing.T) {
					if *withRepoEntryPoint {
						t.Run("with-repo-entrypoint", func(t *testing.T) {
							runTest(t, &remoteTC, "remote", "repo")
						})
					}
					if *withoutRepoEntryPoint {
						t.Run("without-repo-entrypoint", func(t *testing.T) {
							runTest(t, &remoteTC, "remote", "path")
						})
					}
				})
			}
		})
	}
}

func runTest(t *testing.T, tc *e2e.TestCase, mode, entrypoint string) {
	t.Logf("Running test case: %s", tc.Name)
	var err error

	cacheDir := t.TempDir()
	t.Cleanup(func() {
		os.RemoveAll(cacheDir)
	})

	if mode == "local" {
		for _, repo := range tc.Repositories {
			repoDir := filepath.Join(cacheDir, "repos", repo.Owner, repo.Name)
			if err := os.MkdirAll(repoDir, 0755); err != nil {
				t.Fatalf("failed to create repo dir: %v", err)
			}
			cmd := exec.Command("git", "init")
			cmd.Dir = repoDir
			if err := cmd.Run(); err != nil {
				t.Fatalf("failed to git init: %v", err)
			}
			takoFile := filepath.Join(repoDir, "tako.yml")
			content, err := yaml.Marshal(repo.TakoConfig)
			if err != nil {
				t.Fatalf("failed to marshal tako config: %v", err)
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
		}
	} else {
		client, err := e2e.GetClient()
		if err != nil {
			t.Fatalf("failed to get github client: %v", err)
		}
		if err := tc.Setup(client); err != nil {
			t.Fatalf("failed to setup remote test case: %v", err)
		}
		t.Cleanup(func() {
			if err := tc.Cleanup(client); err != nil {
				t.Errorf("failed to cleanup remote test case: %v", err)
			}
		})
		for _, repo := range tc.Repositories {
			repoDir := filepath.Join(cacheDir, "repos", repo.Owner, repo.Name)
			if err := os.MkdirAll(repoDir, 0755); err != nil {
				t.Fatalf("failed to create repo dir: %v", err)
			}
			cmd := exec.Command("git", "clone", repo.CloneURL, repoDir)
			if err := cmd.Run(); err != nil {
				t.Fatalf("failed to git clone: %v", err)
			}
		}
	}

	// Build the tako binary
	takoBinaryDir := t.TempDir()
	t.Cleanup(func() {
		os.RemoveAll(takoBinaryDir)
	})
	takoPath := filepath.Join(takoBinaryDir, "tako")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	projectRoot := findProjectRoot(wd)
	if projectRoot == "" {
		t.Fatal("failed to find project root")
	}
	buildCmd := exec.Command("go", "build", "-o", takoPath, "./cmd/tako")
	buildCmd.Dir = projectRoot
	var buildOut bytes.Buffer
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut
	err = buildCmd.Run()
	if err != nil {
		t.Fatalf("failed to build tako binary: %v\nOutput:\n%s", err, buildOut.String())
	}

	// Run tako graph
	var out bytes.Buffer
	var args []string
	if entrypoint == "repo" {
		args = []string{"graph", "--repo", tc.GetRepoEntryPoint(), "--cache-dir", cacheDir}
	} else {
		rootPath := filepath.Join(cacheDir, "repos", tc.Repositories[0].Owner, tc.Repositories[0].Name)
		args = []string{"graph", "--root", rootPath, "--cache-dir", cacheDir}
		if mode == "local" {
			args = append(args, "--local")
		}
	}

	takoCmd := exec.Command(takoPath, args...)
	takoCmd.Stdout = &out
	takoCmd.Stderr = &out
	err = takoCmd.Run()

	if tc.ExpectedError != "" {
		if err == nil {
			t.Fatalf("expected to fail with error %q, but it succeeded", tc.ExpectedError)
		}
		if !strings.Contains(out.String(), tc.ExpectedError) {
			t.Errorf("expected output to contain %q, got %q", tc.ExpectedError, out.String())
		}
		return
	}

	if err != nil {
		if !strings.Contains(out.String(), getExpectedOutput(tc.Name)) {
			t.Fatalf("failed to run tako graph: %v\nOutput:\n%s", err, out.String())
		}
	}

	// Assert the output
	expected := getExpectedOutput(tc.Name)
	if !strings.Contains(out.String(), expected) {
		if testing.Verbose() {
			t.Logf("Expected output:\n%s", expected)
			t.Logf("Actual output:\n%s", out.String())
		}
		t.Errorf("expected output to contain %q, got %q", expected, out.String())
	}
}

func getExpectedOutput(testCaseName string) string {
	switch testCaseName {
	case "simple-graph":
		return "simple-graph-repo-a\n└── simple-graph-repo-b\n"
	case "complex-graph":
		return "complex-graph-repo-a\n├── complex-graph-repo-b\n│   └── complex-graph-repo-c\n│       └── complex-graph-repo-e\n└── complex-graph-repo-d\n    └── complex-graph-repo-e\n"
	case "deep-graph":
		return "deep-graph-repo-x\n└── deep-graph-repo-y\n    └── deep-graph-repo-z\n"
	case "diamond-dependency-graph":
		return "diamond-dependency-graph-repo-a\n├── diamond-dependency-graph-repo-b\n│   └── diamond-dependency-graph-repo-c\n│       └── diamond-dependency-graph-repo-e\n└── diamond-dependency-graph-repo-d\n    └── diamond-dependency-graph-repo-e\n"
	case "circular-dependency-graph":
		return "circular dependency detected: circular-dependency-graph-repo-circ-a -> circular-dependency-graph-repo-circ-b -> circular-dependency-graph-repo-circ-a"
	default:
		return ""
	}
}
