package e2e

// Step represents a single command and its expected outcome in a TestCase.
type Step struct {
	Name                 string
	Command              string // The command to execute (e.g., "tako", "mvn", "git")
	Args                 []string
	ExpectedOutput       string   // Expected stdout
	AssertOutput         bool     // Whether to perform a strict assertion on the output
	AssertOutputContains []string // Substrings to assert are present in the output
	ExpectedExitCode     int      // Defaults to 0
}

// VerifyFileExists checks if a file exists and optionally verifies its content.
type VerifyFileExists struct {
	FileName        string // Name of the file to check (relative to repo root)
	ShouldExist     bool   // Whether the file should exist
	ExpectedContent string // Expected content (optional, only checked if ShouldExist is true)
}

// Verification defines what to verify after test execution.
type Verification struct {
	Files []VerifyFileExists // Files to check in each repository
}

// TestCase defines a multi-step test to run within an environment.
type TestCase struct {
	Name        string
	Environment string       // The name of the TestEnvironmentDef to use
	ReadOnly    bool         // If true, this test does not modify the environment
	Setup       []Step       // Optional steps to run before the main test
	Test        []Step       // The core test steps
	Verify      Verification // What to verify after execution
}
