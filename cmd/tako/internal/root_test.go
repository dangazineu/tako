package internal

import (
	"bytes"
	"testing"
)

func TestExecute(t *testing.T) {
	cmd := NewRootCmd()
	b := bytes.NewBufferString("")
	cmd.SetOut(b)
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
