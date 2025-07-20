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
			Verify: Verification{
				Files: []VerifyFileExists{
					{
						FileName:        "test.txt",
						ShouldExist:     true,
						ExpectedContent: "hello",
					},
				},
			},
		},
		{
			Name:        "run-dry-run-prevents-execution",
			Environment: "simple-graph",
			ReadOnly:    true,
			Test: []Step{
				{
					Name:         "tako run with dry-run flag",
					Command:      "tako",
					Args:         []string{"run", "--dry-run", "echo 'hello' > dry-run-test.txt"},
					AssertOutput: true,
					ExpectedOutput: `[dry-run] {{.Repo.repo-a}}: echo 'hello' > dry-run-test.txt
[dry-run] {{.Repo.repo-b}}: echo 'hello' > dry-run-test.txt
`,
				},
			},
			Verify: Verification{
				Files: []VerifyFileExists{
					{
						FileName:    "dry-run-test.txt",
						ShouldExist: false,
					},
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
					Name:    "verify lib-a was installed",
					Command: "test",
					Args:    []string{"-f", "${MAVEN_REPO_DIR}/com/tako/lib-a/1.0.0/lib-a-1.0.0.jar"},
				},
				{
					Name:    "verify lib-b was installed",
					Command: "test",
					Args:    []string{"-f", "${MAVEN_REPO_DIR}/com/tako/lib-b/1.0.0/lib-b-1.0.0.jar"},
				},
			},
			Test: []Step{
				{
					Name:    "introduce incompatible change",
					Command: "cp",
					Args:    []string{"test/e2e/templates/java-binary-incompatibility/repo-a/src/main/java/com/tako/lib_a/Producer_modified.java", "{{.Repo.repo-a}}/src/main/java/com/tako/lib_a/Producer.java"},
				},
				{
					Name:    "naive partial rebuild only repo-a",
					Command: "mvn",
					Args:    []string{"-f", "{{.Repo.repo-a}}/pom.xml", "clean", "install", "-Dmaven.repo.local=${MAVEN_REPO_DIR}"},
				},
				{
					Name:             "verify downstream failure",
					Command:          "mvn",
					Args:             []string{"-f", "{{.Repo.repo-c}}/pom.xml", "clean", "test", "-Dmaven.repo.local=${MAVEN_REPO_DIR}"},
					ExpectedExitCode: 1,
					AssertOutputContains: []string{
						"IncompatibleClassChangeError",
					},
				},
				{
					Name:    "tako run rebuilds entire dependency chain",
					Command: "tako",
					Args:    []string{"run", "mvn clean install -Dmaven.repo.local=${MAVEN_REPO_DIR}"},
				},
				{
					Name:    "verify downstream success",
					Command: "mvn",
					Args:    []string{"-f", "{{.Repo.repo-c}}/pom.xml", "clean", "test", "-Dmaven.repo.local=${MAVEN_REPO_DIR}"},
				},
			},
		},
	}
}
