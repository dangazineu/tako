//go:build e2e

package e2e

func GetTestCases() []TestCase {
	return []TestCase{
		{
			Name:        "graph-simple",
			Environment: "simple-graph",
			ReadOnly:    true,
			Test: []Step{
				{
					Name:         "tako graph",
					Command:      "tako",
					Args:         []string{"graph"},
					AssertOutput: true,
					ExpectedOutput: `{{.Repo.repo-a}}
└── {{.Repo.repo-b}}
`,
				},
			},
		},
		{
			Name:        "run-touch-command",
			Environment: "simple-graph",
			ReadOnly:    false,
			Test: []Step{
				{
					Name:    "tako run create file",
					Command: "tako",
					Args:    []string{"run", "echo 'hello' > test.txt"},
				},
			},
		},
		{
			Name:        "java-binary-incompatibility",
			Environment: "java-binary-incompatibility",
			ReadOnly:    false,
			Setup: []Step{
				{
					Name:    "initial clean build",
					Command: "tako",
					Args:    []string{"run", "mvn clean install -Dmaven.repo.local=${MAVEN_REPO_DIR}"},
				},
				{
					Name:    "introduce incompatible change",
					Command: "cp",
					Args: []string{
						"test/e2e/templates/java-binary-incompatibility/repo-a/src/main/java/com/tako/lib_a/SubClass_modified.java",
						"{{.Repo.repo-a}}/src/main/java/com/tako/lib_a/SubClass.java",
					},
				},
				{
					Name:    "naive partial rebuild",
					Command: "mvn",
					Args:    []string{"-f", "{{.Repo.repo-a}}/pom.xml", "clean", "install", "-Dmaven.repo.local=${MAVEN_REPO_DIR}"},
				},
			},
			Test: []Step{
				{
					Name:    "the tako solution",
					Command: "tako",
					Args:    []string{"run", "mvn clean install -Dmaven.repo.local=${MAVEN_REPO_DIR}"},
				},
			},
		},
	}
}
