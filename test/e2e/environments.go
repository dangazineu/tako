package e2e

// FileDef represents a file to be created in a repository from a template.
type FileDef struct {
	Path     string // Relative path within the repository
	Template string // Name of the template file (e.g., "java-library/pom.xml")
}

// RepositoryDef defines a repository within an environment.
type RepositoryDef struct {
	Name         string
	Branch       string
	Files        []FileDef
	Dependencies []string
}

// TestEnvironmentDef defines the repository layout for a test world.
type TestEnvironmentDef struct {
	Name         string
	Repositories []RepositoryDef
}

func GetEnvironments(owner string) map[string]TestEnvironmentDef {
	return map[string]TestEnvironmentDef{
		"simple-graph": {
			Name: "simple-graph",
			Repositories: []RepositoryDef{
				{Name: "repo-a", Branch: "main", Dependencies: []string{"repo-b"}},
				{Name: "repo-b", Branch: "main"},
			},
		},
		"complex-graph": {
			Name: "complex-graph",
			Repositories: []RepositoryDef{
				{Name: "repo-a", Branch: "main", Dependencies: []string{"repo-b", "repo-d"}},
				{Name: "repo-b", Branch: "main", Dependencies: []string{"repo-c"}},
				{Name: "repo-c", Branch: "main", Dependencies: []string{"repo-e"}},
				{Name: "repo-d", Branch: "main", Dependencies: []string{"repo-e"}},
				{Name: "repo-e", Branch: "main"},
			},
		},
		"deep-graph": {
			Name: "deep-graph",
			Repositories: []RepositoryDef{
				{Name: "repo-x", Branch: "main", Dependencies: []string{"repo-y"}},
				{Name: "repo-y", Branch: "main", Dependencies: []string{"repo-z"}},
				{Name: "repo-z", Branch: "main"},
			},
		},
		"diamond-dependency-graph": {
			Name: "diamond-dependency-graph",
			Repositories: []RepositoryDef{
				{Name: "repo-a", Branch: "main", Dependencies: []string{"repo-b", "repo-d"}},
				{Name: "repo-b", Branch: "main", Dependencies: []string{"repo-c"}},
				{Name: "repo-c", Branch: "main", Dependencies: []string{"repo-e"}},
				{Name: "repo-d", Branch: "main", Dependencies: []string{"repo-e"}},
				{Name: "repo-e", Branch: "main"},
			},
		},
		"circular-dependency-graph": {
			Name: "circular-dependency-graph",
			Repositories: []RepositoryDef{
				{Name: "repo-circ-a", Branch: "main", Dependencies: []string{"repo-circ-b"}},
				{Name: "repo-circ-b", Branch: "main", Dependencies: []string{"repo-circ-a"}},
			},
		},
		"java-binary-incompatibility": {
			Name: "java-binary-incompatibility",
			Repositories: []RepositoryDef{
				{
					Name:         "repo-a",
					Branch:       "main",
					Dependencies: []string{"repo-b"},
					Files: []FileDef{
						{Path: "pom.xml", Template: "java-binary-incompatibility/repo-a/pom.xml"},
						{Path: "src/main/java/com/tako/lib_a/Producer.java", Template: "java-binary-incompatibility/repo-a/src/main/java/com/tako/lib_a/Producer.java"},
					},
				},
				{
					Name:         "repo-b",
					Branch:       "main",
					Dependencies: []string{"repo-c"},
					Files: []FileDef{
						{Path: "pom.xml", Template: "java-binary-incompatibility/repo-b/pom.xml"},
						{Path: "src/main/java/com/tako/lib_b/Consumer.java", Template: "java-binary-incompatibility/repo-b/src/main/java/com/tako/lib_b/Consumer.java"},
					},
				},
				{
					Name:   "repo-c",
					Branch: "main",
					Files: []FileDef{
						{Path: "pom.xml", Template: "java-binary-incompatibility/repo-c/pom.xml"},
						{Path: "src/test/java/com/tako/app_c/AppTest.java", Template: "java-binary-incompatibility/repo-c/src/test/java/com/tako/app_c/AppTest.java"},
					},
				},
			},
		},
		"single-repo-workflow": {
			Name: "single-repo-workflow",
			Repositories: []RepositoryDef{
				{
					Name:   "test-repo",
					Branch: "main",
					Files: []FileDef{
						{Path: "scripts/prepare.sh", Template: "single-repo-workflow/scripts/prepare.sh"},
						{Path: "scripts/long-process.sh", Template: "single-repo-workflow/scripts/long-process.sh"},
						{Path: "scripts/finalize.sh", Template: "single-repo-workflow/scripts/finalize.sh"},
						{Path: "test-file.txt", Template: "single-repo-workflow/test-file.txt"},
					},
				},
			},
		},
		"malformed-config": {
			Name: "malformed-config",
			Repositories: []RepositoryDef{
				{
					Name:   "bad-config-repo",
					Branch: "main",
					Files: []FileDef{
						{Path: "malformed-tako.yml", Template: "malformed-config/malformed-tako.yml"},
					},
				},
			},
		},
		"fan-out-test": {
			Name: "fan-out-test",
			Repositories: []RepositoryDef{
				{
					Name:   "publisher-repo",
					Branch: "main",
					Files: []FileDef{
						{Path: "tako.yml", Template: "fan-out-test/publisher-repo/tako.yml"},
					},
				},
				{
					Name:   "subscriber-repo-a",
					Branch: "main",
					Files: []FileDef{
						{Path: "tako.yml", Template: "fan-out-test/subscriber-repo-a/tako.yml"},
					},
				},
				{
					Name:   "subscriber-repo-b",
					Branch: "main",
					Files: []FileDef{
						{Path: "tako.yml", Template: "fan-out-test/subscriber-repo-b/tako.yml"},
					},
				},
			},
		},
		"local-go-ci-pipeline": {
			Name: "local-go-ci-pipeline",
			Repositories: []RepositoryDef{
				{
					Name:   "go-app",
					Branch: "main",
					Files: []FileDef{
						{Path: "main.go", Template: "local-go-ci-pipeline/main.go"},
						{Path: "go.mod", Template: "local-go-ci-pipeline/go.mod.template"},
						{Path: "Dockerfile", Template: "local-go-ci-pipeline/Dockerfile"},
						{Path: "tako.yml", Template: "local-go-ci-pipeline/tako.yml"},
					},
				},
			},
		},
		"local-go-ci-pipeline-lint-failure": {
			Name: "local-go-ci-pipeline-lint-failure",
			Repositories: []RepositoryDef{
				{
					Name:   "go-app",
					Branch: "main",
					Files: []FileDef{
						{Path: "main.go", Template: "local-go-ci-pipeline-lint-failure/main.go"},
						{Path: "go.mod", Template: "local-go-ci-pipeline-lint-failure/go.mod.template"},
						{Path: "Dockerfile", Template: "local-go-ci-pipeline-lint-failure/Dockerfile"},
						{Path: "tako.yml", Template: "local-go-ci-pipeline-lint-failure/tako.yml"},
					},
				},
			},
		},
		"local-go-ci-pipeline-build-failure": {
			Name: "local-go-ci-pipeline-build-failure",
			Repositories: []RepositoryDef{
				{
					Name:   "go-app",
					Branch: "main",
					Files: []FileDef{
						{Path: "main.go", Template: "local-go-ci-pipeline-build-failure/main.go"},
						{Path: "go.mod", Template: "local-go-ci-pipeline-build-failure/go.mod.template"},
						{Path: "Dockerfile", Template: "local-go-ci-pipeline-build-failure/Dockerfile"},
						{Path: "tako.yml", Template: "local-go-ci-pipeline-build-failure/tako.yml"},
					},
				},
			},
		},
		"local-go-ci-pipeline-package-failure": {
			Name: "local-go-ci-pipeline-package-failure",
			Repositories: []RepositoryDef{
				{
					Name:   "go-app",
					Branch: "main",
					Files: []FileDef{
						{Path: "main.go", Template: "local-go-ci-pipeline-package-failure/main.go"},
						{Path: "go.mod", Template: "local-go-ci-pipeline-package-failure/go.mod.template"},
						{Path: "Dockerfile", Template: "local-go-ci-pipeline-package-failure/Dockerfile"},
						{Path: "tako.yml", Template: "local-go-ci-pipeline-package-failure/tako.yml"},
					},
				},
			},
		},
	}
}
