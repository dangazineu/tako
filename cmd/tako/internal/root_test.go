package internal

import (
	"testing"
)

func TestExecute(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute root command: %v", err)
	}
}
