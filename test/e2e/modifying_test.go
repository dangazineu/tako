//go:build e2e && modifying

package e2e

var modifyingTestCases = []TestCase{
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
				Args:    []string{"run", "mvn", "clean", "install", "-Dmaven.repo.local=${MAVEN_REPO_DIR}"},
			},
			{
				Name:    "verify initial state",
				Command: "mvn",
				Args:    []string{"-f", "{{.Repo.repo-c}}/pom.xml", "test", "-Dmaven.repo.local=${MAVEN_REPO_DIR}"},
			},
			{
				Name:    "introduce incompatible change",
				Command: "mv",
				Args: []string{
					"{{.Repo.repo-a}}/src/main/java/com/tako/lib_a/SubClass_modified.java",
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
				Name:             "verify broken state",
				Command:          "mvn",
				Args:             []string{"-f", "{{.Repo.repo-c}}/pom.xml", "test", "-Dmaven.repo.local=${MAVEN_REPO_DIR}"},
				ExpectedExitCode: 1,
				AssertOutput:     true,
				ExpectedOutput:   "java.lang.NoSuchMethodError",
			},
			{
				Name:    "the tako solution",
				Command: "tako",
				Args:    []string{"run", "mvn", "clean", "install", "-Dmaven.repo.local=${MAVEN_REPO_DIR}"},
			},
			{
				Name:             "verify final success",
				Command:          "mvn",
				Args:             []string{"-f", "{{.Repo.repo-c}}/pom.xml", "test", "-Dmaven.repo.local=${MAVEN_REPO_DIR}"},
				ExpectedExitCode: 0,
			},
		},
	},
}
