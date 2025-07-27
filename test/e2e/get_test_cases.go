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
		{
			Name:        "exec-basic-workflow",
			Environment: "simple-graph",
			ReadOnly:    true,
			Test: []Step{
				{
					Name:    "tako exec basic workflow",
					Command: "tako",
					Args:    []string{"exec", "test-workflow", "--inputs.environment=dev"},
					AssertOutputContains: []string{
						"Executing workflow 'test-workflow'",
						"Inputs:",
						"environment: dev",
					},
				},
			},
		},
		{
			Name:        "exec-advanced-input-validation",
			Environment: "single-repo-workflow",
			ReadOnly:    true,
			Test: []Step{
				{
					Name:    "tako exec with valid enum input",
					Command: "tako",
					Args:    []string{"exec", "advanced-input-workflow", "--inputs.environment=staging", "--inputs.version=2.0.0"},
					AssertOutputContains: []string{
						"Executing workflow 'advanced-input-workflow'",
						"environment: staging",
						"version: 2.0.0",
						"Success: true",
						"validate_inputs",
						"process_with_templates",
						"final_step",
					},
				},
				{
					Name:             "tako exec with invalid enum input should fail",
					Command:          "tako",
					Args:             []string{"exec", "advanced-input-workflow", "--inputs.environment=invalid"},
					ExpectedExitCode: 1,
					AssertOutputContains: []string{
						"workflow execution failed",
						"not in allowed values",
					},
				},
				{
					Name:             "tako exec missing required input should fail",
					Command:          "tako",
					Args:             []string{"exec", "advanced-input-workflow", "--inputs.version=1.0.0"},
					ExpectedExitCode: 1,
					AssertOutputContains: []string{
						"required input 'environment' not provided",
					},
				},
			},
		},
		{
			Name:        "exec-step-output-passing",
			Environment: "single-repo-workflow",
			ReadOnly:    true,
			Test: []Step{
				{
					Name:    "tako exec step output workflow",
					Command: "tako",
					Args:    []string{"exec", "step-output-workflow"},
					AssertOutputContains: []string{
						"Executing workflow 'step-output-workflow'",
						"Success: true",
						"step1",
						"step2",
						"step3",
					},
				},
			},
		},
		{
			Name:        "exec-template-variable-resolution",
			Environment: "single-repo-workflow",
			ReadOnly:    true,
			Test: []Step{
				{
					Name:    "tako exec template variables with default values",
					Command: "tako",
					Args:    []string{"exec", "template-variable-workflow"},
					AssertOutputContains: []string{
						"Executing workflow 'template-variable-workflow'",
						"Success: true",
						"test_variables",
						"test_security_functions",
					},
				},
				{
					Name:    "tako exec template variables with custom values",
					Command: "tako",
					Args:    []string{"exec", "template-variable-workflow", "--inputs.message=Custom Message", "--inputs.count=10"},
					AssertOutputContains: []string{
						"Executing workflow 'template-variable-workflow'",
						"message: Custom Message",
						"count: 10",
						"Success: true",
					},
				},
				{
					Name:    "tako exec template security functions",
					Command: "tako",
					Args:    []string{"exec", "template-variable-workflow", "--inputs.message=test'quote\"json&html"},
					AssertOutputContains: []string{
						"Executing workflow 'template-variable-workflow'",
						"Success: true",
						"test_variables",
						"test_security_functions",
					},
				},
			},
		},
		{
			Name:        "exec-error-handling",
			Environment: "single-repo-workflow",
			ReadOnly:    true,
			Test: []Step{
				{
					Name:             "tako exec workflow with step failure",
					Command:          "tako",
					Args:             []string{"exec", "error-handling-workflow"},
					ExpectedExitCode: 1,
					AssertOutputContains: []string{
						"Executing workflow 'error-handling-workflow'",
						"workflow execution failed",
						"success_step",
						"failure_step",
					},
				},
			},
		},
		{
			Name:        "exec-long-running-workflow",
			Environment: "single-repo-workflow",
			ReadOnly:    false,
			Test: []Step{
				{
					Name:    "tako exec long-running workflow",
					Command: "tako",
					Args:    []string{"exec", "long-running-workflow"},
					AssertOutputContains: []string{
						"Executing workflow 'long-running-workflow'",
						"Success: true",
						"prepare",
						"long_process",
						"finalize",
					},
				},
			},
			Verify: Verification{
				Files: []VerifyFileExists{
					{
						FileName:        "preparation.log",
						ShouldExist:     true,
						ExpectedContent: "Workflow preparation complete",
					},
					{
						FileName:        "long-process.log",
						ShouldExist:     true,
						ExpectedContent: "Long process completed successfully",
					},
					{
						FileName:        "finalization.log",
						ShouldExist:     true,
						ExpectedContent: "Workflow finalization complete",
					},
				},
			},
		},
		{
			Name:        "exec-dry-run-mode",
			Environment: "single-repo-workflow",
			ReadOnly:    true,
			Test: []Step{
				{
					Name:    "tako exec with dry-run flag",
					Command: "tako",
					Args:    []string{"exec", "long-running-workflow", "--dry-run"},
					AssertOutputContains: []string{
						"Executing workflow 'long-running-workflow'",
						"[dry-run]",
					},
				},
			},
			Verify: Verification{
				Files: []VerifyFileExists{
					{
						FileName:    "preparation.log",
						ShouldExist: false,
					},
					{
						FileName:    "long-process.log",
						ShouldExist: false,
					},
					{
						FileName:    "finalization.log",
						ShouldExist: false,
					},
				},
			},
		},
		{
			Name:        "exec-debug-mode",
			Environment: "single-repo-workflow",
			ReadOnly:    true,
			Test: []Step{
				{
					Name:    "tako exec with debug flag",
					Command: "tako",
					Args:    []string{"exec", "step-output-workflow", "--debug"},
					AssertOutputContains: []string{
						"Executing workflow 'step-output-workflow'",
						"Debug mode enabled",
					},
				},
			},
		},
		{
			Name:        "exec-workflow-not-found",
			Environment: "single-repo-workflow",
			ReadOnly:    true,
			Test: []Step{
				{
					Name:             "tako exec non-existent workflow should fail",
					Command:          "tako",
					Args:             []string{"exec", "non-existent-workflow"},
					ExpectedExitCode: 1,
					AssertOutputContains: []string{
						"workflow 'non-existent-workflow' not found",
					},
				},
			},
		},
		{
			Name:        "exec-security-functions",
			Environment: "single-repo-workflow",
			ReadOnly:    true,
			Test: []Step{
				{
					Name:    "tako exec template security functions with special characters",
					Command: "tako",
					Args:    []string{"exec", "template-variable-workflow", "--inputs.message=test'; rm -rf /; echo 'pwned"},
					AssertOutputContains: []string{
						"Executing workflow 'template-variable-workflow'",
						"Success: true",
						"test_security_functions",
					},
				},
			},
		},
	}
}
