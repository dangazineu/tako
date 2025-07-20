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
	}
}
