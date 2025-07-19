package internal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunCmd(t *testing.T) {
	tmpDir := t.TempDir()
	takoFile := filepath.Join(tmpDir, "tako.yml")
	err := os.WriteFile(takoFile, []byte("version: 1.0\nmetadata:\n  name: test\ndependents: []"), 0644)
	if err != nil {
		t.Fatalf("failed to write dummy tako.yml: %v", err)
	}

	cmd := NewRunCmd()
	b := bytes.NewBufferString("")
	cmd.SetOut(b)
	cmd.SetErr(b)
	cmd.SetArgs([]string{"--local", "--root", tmpDir, "echo 'hello'"})
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}