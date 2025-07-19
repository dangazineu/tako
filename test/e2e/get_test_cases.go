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
	}
}
