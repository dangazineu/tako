package internal

import (
	"bytes"
	"runtime/debug"
	"testing"
)

func TestVersionCmd(t *testing.T) {
	// Execute the version command
	b := bytes.NewBufferString("")
	errb := bytes.NewBufferString("")
	cmd := NewVersionCmd()
	cmd.SetOut(b)
	cmd.SetErr(errb)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute version command: %v", err)
	}

	// Check the output
	if b.String() == "" && errb.String() == "" {
		t.Errorf("expected output to not be empty")
	}
}

func TestDeriveVersion(t *testing.T) {
	t.Run("with version", func(t *testing.T) {
		info := &debug.BuildInfo{
			Main: debug.Module{
				Version: "v1.2.3",
			},
		}
		version, err := deriveVersionFromInfo(info)
		if err != nil {
			t.Fatalf("failed to derive version: %v", err)
		}
		if version != "v1.2.3" {
			t.Errorf("expected version to be 'v1.2.3', got %q", version)
		}
	})

	t.Run("with pseudo version", func(t *testing.T) {
		info := &debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef123456"},
				{Key: "vcs.time", Value: "2025-07-15T12:00:00Z"},
			},
		}
		version, err := deriveVersionFromInfo(info)
		if err != nil {
			t.Fatalf("failed to derive version: %v", err)
		}
		expected := "v0.0.0-20250715120000-abcdef123456"
		if version != expected {
			t.Errorf("expected version to be %q, got %q", expected, version)
		}
	})

	t.Run("no version", func(t *testing.T) {
		info := &debug.BuildInfo{}
		_, err := deriveVersionFromInfo(info)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
	})
}
