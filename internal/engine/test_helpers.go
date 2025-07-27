package engine

import (
	"os"
	"testing"

	"github.com/dangazineu/tako/internal/config"
)

// createTestWorkflow creates a test workflow for testing purposes.
func createTestWorkflow() config.Workflow {
	return config.Workflow{
		Name: "test-workflow",
		Inputs: map[string]config.WorkflowInput{
			"environment": {
				Type:     "string",
				Required: true,
				Validation: config.WorkflowInputValidation{
					Enum: []string{"dev", "staging", "prod"},
				},
			},
			"version": {
				Type:    "string",
				Default: "1.0.0",
			},
		},
		Steps: []config.WorkflowStep{
			{
				ID:  "validate_input",
				Run: "echo 'Deploying to {{ .Inputs.environment }}'",
			},
			{
				ID:  "process_output",
				Run: "echo 'processed-{{ .Steps.validate_input.result }}'",
				Produces: &config.WorkflowStepProduces{
					Outputs: map[string]string{
						"final_result": "from_stdout",
					},
				},
			},
		},
	}
}

// createTestTakoConfig creates a test tako.yml file for testing.
func createTestTakoConfig(t *testing.T, filePath string) {
	content := `version: 0.1.0
artifacts:
  default:
    path: "."
    ecosystem: "generic"
workflows:
  test-workflow:
    inputs:
      environment:
        type: string
        required: true
        validation:
          enum: ["dev", "staging", "prod"]
      version:
        type: string
        default: "1.0.0"
    steps:
      - id: validate_input
        run: echo 'Deploying to {{ .Inputs.environment }}'
      - id: process_output
        run: echo 'processed-{{ .Steps.validate_input.result }}'
        produces:
          outputs:
            final_result: from_stdout
subscriptions: []
`

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test tako.yml: %v", err)
	}
}
