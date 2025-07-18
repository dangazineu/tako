package internal

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompletionCmd(t *testing.T) {
	rootCmd := NewRootCmd()
	rootCmd.AddCommand(NewCompletionCmd())

	testCases := []struct {
		name      string
		args      []string
		expectErr bool
	}{
		{
			name:      "bash completion",
			args:      []string{"completion", "bash"},
			expectErr: false,
		},
		{
			name:      "zsh completion",
			args:      []string{"completion", "zsh"},
			expectErr: false,
		},
		{
			name:      "fish completion",
			args:      []string{"completion", "fish"},
			expectErr: false,
		},
		{
			name:      "powershell completion",
			args:      []string{"completion", "powershell"},
			expectErr: false,
		},
		{
			name:      "invalid shell",
			args:      []string{"completion", "invalid"},
			expectErr: true,
		},
		{
			name:      "no shell",
			args:      []string{"completion"},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			rootCmd.SetOut(&out)
			rootCmd.SetErr(&out)
			rootCmd.SetArgs(tc.args)

			err := rootCmd.Execute()

			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if !strings.Contains(out.String(), "comp") {
					t.Errorf("Expected output to contain a completion script, but it did not")
				}
			}
		})
	}
}
