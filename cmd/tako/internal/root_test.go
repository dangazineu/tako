package internal

import (
	"bytes"
	"testing"
)

func TestExecute(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute root command: %v", err)
	}
}
