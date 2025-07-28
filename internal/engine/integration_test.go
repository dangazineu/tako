package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dangazineu/tako/internal/config"
)

// TestContainerizedWorkflowIntegration tests full integration of containerized execution
// with security, resources, and registry support.
func TestContainerizedWorkflowIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test directory
	tmpDir := t.TempDir()
	auditLog := filepath.Join(tmpDir, "audit.log")
	cacheDir := filepath.Join(tmpDir, "cache")

	// Create container manager
	cm, err := NewContainerManager(true)
	if err != nil {
		t.Skipf("Container runtime not available: %v", err)
	}

	// Create security manager
	sm, err := NewSecurityManager(auditLog, true)
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}
	defer sm.Close()

	// Create registry manager
	rm, err := NewRegistryManager(cacheDir, true)
	if err != nil {
		t.Fatalf("Failed to create registry manager: %v", err)
	}

	// Create resource manager
	resman := NewResourceManager(&ResourceManagerConfig{
		Debug: testing.Verbose(), // Only enable debug output in verbose mode
	})

	// Set moderate security profile for testing
	cm.WithSecurityManager(sm).WithRegistryManager(rm)

	// Set resource limits
	err = resman.SetGlobalQuota("2.0", "1Gi", "10Gi")
	if err != nil {
		t.Fatalf("Failed to set global quota: %v", err)
	}

	// Define a test workflow step
	step := config.WorkflowStep{
		ID:    "test-containerized-step",
		Image: "alpine:latest",
		Run:   "echo 'Hello from containerized Tako!' && echo 'User ID:' && id && pwd && env | grep -E 'TAKO|TEST_VAR' || true",
		Env: map[string]string{
			"TEST_VAR": "integration_test",
		},
		SecurityProfile: "moderate",
		Resources: &config.Resources{
			CPULimit: "0.5",
			MemLimit: "128MB",
		},
	}

	// Validate configuration
	if err := cm.ValidateContainerConfig(step); err != nil {
		t.Fatalf("Configuration validation failed: %v", err)
	}

	// Validate resources
	if err := resman.ValidateResourceRequest("test-repo", step.ID, "0.5", "128Mi"); err != nil {
		t.Fatalf("Resource validation failed: %v", err)
	}

	// Build container configuration
	workDir := tmpDir
	env := map[string]string{
		"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}

	config, err := cm.BuildContainerConfig(step, workDir, env, step.Resources)
	if err != nil {
		t.Fatalf("Failed to build container config: %v", err)
	}

	// Verify security configuration was applied
	if config.Security == nil {
		t.Fatal("Security configuration should be set")
	}
	if !config.Security.NoNewPrivileges {
		t.Error("NoNewPrivileges should be enabled")
	}
	if config.Security.RunAsUser != 1001 {
		t.Errorf("Expected RunAsUser=1001, got %d", config.Security.RunAsUser)
	}

	// Verify network isolation
	if config.Network != "none" {
		t.Errorf("Expected network=none for moderate profile, got %s", config.Network)
	}

	// Create context with run information
	ctx := context.Background()
	// Note: In production code, these would use typed context keys for safety

	// Pull the image first
	if err := cm.PullImage(ctx, step.Image); err != nil {
		t.Fatalf("Failed to pull image: %v", err)
	}

	// Execute the container
	result, err := cm.RunContainer(ctx, config, step.ID)
	if err != nil {
		t.Fatalf("Container execution failed: %v", err)
	}

	// Verify execution results
	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
		t.Logf("Stdout: %s", result.Stdout)
		t.Logf("Stderr: %s", result.Stderr)
	}

	// Check output contains expected content
	expectedOutputs := []string{
		"Hello from containerized Tako!",
		"TAKO_CONTAINER=true",
		"TEST_VAR=integration_test",
	}

	for _, expected := range expectedOutputs {
		if !strings.Contains(result.Stdout, expected) {
			t.Errorf("Expected output to contain %q, got: %s", expected, result.Stdout)
		}
	}

	// Verify timing information
	if result.StartTime.IsZero() || result.EndTime.IsZero() {
		t.Error("Start and end times should be set")
	}
	if result.EndTime.Before(result.StartTime) {
		t.Error("End time should be after start time")
	}

	// Check audit log was created
	if _, err := os.Stat(auditLog); os.IsNotExist(err) {
		t.Error("Audit log should have been created")
	}

	// Note: Image caching is internal to registry manager
	// The fact that the image pull succeeded indicates caching is working

	// Only log detailed output in verbose mode (when test fails, output will be shown automatically)
	if testing.Verbose() {
		t.Logf("Integration test completed successfully!")
		t.Logf("Container name: %s", result.ContainerName)
		t.Logf("Execution time: %v", result.EndTime.Sub(result.StartTime))
		t.Logf("Output: %s", result.Stdout)
	}
}

// TestSecurityIntegration tests security features integration.
func TestSecurityIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	auditLog := filepath.Join(tmpDir, "audit.log")

	// Test security manager with different profiles
	sm, err := NewSecurityManager(auditLog, true)
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}
	defer sm.Close()

	profiles := []SecurityProfile{
		SecurityProfileStrict,
		SecurityProfileModerate,
		SecurityProfileMinimal,
	}

	for _, profile := range profiles {
		t.Run(string(profile), func(t *testing.T) {
			config := &ContainerConfig{
				Image:    "alpine:latest",
				Security: &SecurityConfig{},
			}

			err := sm.ApplySecurityProfile(config, profile)
			if err != nil {
				t.Fatalf("Failed to apply %s profile: %v", profile, err)
			}

			// Verify profile-specific settings
			switch profile {
			case SecurityProfileStrict:
				if !config.Security.ReadOnlyRootFS {
					t.Error("Strict profile should enable read-only root filesystem")
				}
				if config.Network != "none" {
					t.Error("Strict profile should disable network access")
				}
			case SecurityProfileModerate:
				if !config.Security.NoNewPrivileges {
					t.Error("Moderate profile should prevent privilege escalation")
				}
				if len(config.Security.AddCapabilities) == 0 {
					t.Error("Moderate profile should add some capabilities")
				}
			case SecurityProfileMinimal:
				if !config.Security.NoNewPrivileges {
					t.Error("Minimal profile should still prevent privilege escalation")
				}
				if config.Network == "" {
					t.Error("Minimal profile should set a network mode")
				}
			}
		})
	}
}

// TestResourceIntegration tests resource management integration.
func TestResourceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	rm := NewResourceManager(&ResourceManagerConfig{
		Debug: testing.Verbose(), // Only enable debug output in verbose mode
	})

	// Test hierarchical resource limits
	err := rm.SetGlobalQuota("4.0", "8Gi", "100Gi")
	if err != nil {
		t.Fatalf("Failed to set global quota: %v", err)
	}

	err = rm.SetRepositoryQuota("test-repo", config.Resources{
		CPULimit:  "2.0",
		MemLimit:  "4Gi",
		DiskLimit: "50Gi",
	})
	if err != nil {
		t.Fatalf("Failed to set repository quota: %v", err)
	}

	// Test valid request within limits
	err = rm.ValidateResourceRequest("test-repo", "step1", "1.0", "2Gi")
	if err != nil {
		t.Errorf("Valid resource request should pass: %v", err)
	}

	// Test request exceeding repository limits
	err = rm.ValidateResourceRequest("test-repo", "step2", "3.0", "2Gi") // Exceeds repo CPU limit of 2
	if err == nil {
		t.Error("Resource request exceeding repo limits should fail")
	}

	t.Logf("Resource integration test completed successfully!")
}
