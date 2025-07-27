package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewSecurityManager(t *testing.T) {
	tmpDir := t.TempDir()
	auditLog := filepath.Join(tmpDir, "audit.log")

	sm, err := NewSecurityManager(auditLog, true)
	if err != nil {
		t.Fatalf("NewSecurityManager() failed: %v", err)
	}
	defer sm.Close()

	// Verify default settings
	if !sm.enableAudit {
		t.Error("Audit should be enabled by default")
	}

	if sm.volumeRestrictions == nil {
		t.Error("Volume restrictions should be initialized")
	}

	if sm.networkPolicy == nil {
		t.Error("Network policy should be initialized")
	}

	// Check default blocked paths
	blockedPaths := []string{"/etc", "/sys", "/proc", "/dev", "/root", "/home", "/var/run/docker.sock"}
	for _, path := range blockedPaths {
		found := false
		for _, blocked := range sm.volumeRestrictions.BlockedPaths {
			if blocked == path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Path %s should be in blocked paths", path)
		}
	}
}

func TestSecurityAuditor_LogEvent(t *testing.T) {
	tmpDir := t.TempDir()
	auditLog := filepath.Join(tmpDir, "audit.log")

	auditor, err := NewSecurityAuditor(auditLog, true)
	if err != nil {
		t.Fatalf("NewSecurityAuditor() failed: %v", err)
	}
	defer auditor.writer.(io.Closer).Close()

	// Log a test event
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "test_event",
		RunID:     "test-run-123",
		StepID:    "test-step-456",
		User:      "test-user",
		Action:    "test-action",
		Resource:  "test-resource",
		Result:    "success",
		Details: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	err = auditor.LogEvent(event)
	if err != nil {
		t.Fatalf("LogEvent() failed: %v", err)
	}

	// Read the log file
	content, err := os.ReadFile(auditLog)
	if err != nil {
		t.Fatalf("Failed to read audit log: %v", err)
	}

	// Verify log content
	logStr := string(content)
	if !strings.Contains(logStr, `"type":"test_event"`) {
		t.Error("Log should contain event type")
	}
	if !strings.Contains(logStr, `"run_id":"test-run-123"`) {
		t.Error("Log should contain run ID")
	}
	if !strings.Contains(logStr, `"key1":"value1"`) {
		t.Error("Log should contain event details")
	}
}

func TestSecurityManager_ValidateVolumeMounts(t *testing.T) {
	tmpDir := t.TempDir()
	sm, err := NewSecurityManager(filepath.Join(tmpDir, "audit.log"), false)
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}
	defer sm.Close()

	tests := []struct {
		name    string
		volumes []VolumeMount
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid workspace mount",
			volumes: []VolumeMount{
				{Source: "/workspace/test", Destination: "/app", ReadOnly: false},
			},
			wantErr: false,
		},
		{
			name: "valid tmp mount",
			volumes: []VolumeMount{
				{Source: "/tmp/test", Destination: "/tmp/app", ReadOnly: false},
			},
			wantErr: false,
		},
		{
			name: "blocked etc mount",
			volumes: []VolumeMount{
				{Source: "/etc/passwd", Destination: "/app/passwd", ReadOnly: true},
			},
			wantErr: true,
			errMsg:  "restricted path /etc",
		},
		{
			name: "blocked docker socket",
			volumes: []VolumeMount{
				{Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock", ReadOnly: true},
			},
			wantErr: true,
			errMsg:  "restricted path /var/run/docker.sock",
		},
		{
			name: "too many volumes",
			volumes: []VolumeMount{
				{Source: "/workspace/1", Destination: "/1", ReadOnly: false},
				{Source: "/workspace/2", Destination: "/2", ReadOnly: false},
				{Source: "/workspace/3", Destination: "/3", ReadOnly: false},
				{Source: "/workspace/4", Destination: "/4", ReadOnly: false},
				{Source: "/workspace/5", Destination: "/5", ReadOnly: false},
				{Source: "/workspace/6", Destination: "/6", ReadOnly: false},
			},
			wantErr: true,
			errMsg:  "too many volume mounts",
		},
		{
			name: "usr must be read-only",
			volumes: []VolumeMount{
				{Source: "/usr/local", Destination: "/usr/local", ReadOnly: false},
			},
			wantErr: true,
			errMsg:  "must be read-only",
		},
		{
			name: "usr read-only allowed",
			volumes: []VolumeMount{
				{Source: "/usr/local", Destination: "/usr/local", ReadOnly: true},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sm.ValidateVolumeMounts(tt.volumes)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVolumeMounts() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateVolumeMounts() error = %v, should contain %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestSecurityManager_ApplySecurityProfile(t *testing.T) {
	tmpDir := t.TempDir()
	sm, err := NewSecurityManager(filepath.Join(tmpDir, "audit.log"), false)
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}
	defer sm.Close()

	tests := []struct {
		name     string
		profile  SecurityProfile
		validate func(*ContainerConfig) error
	}{
		{
			name:    "strict profile",
			profile: SecurityProfileStrict,
			validate: func(config *ContainerConfig) error {
				if !config.Security.NoNewPrivileges {
					return fmt.Errorf("NoNewPrivileges should be true")
				}
				if !config.Security.ReadOnlyRootFS {
					return fmt.Errorf("ReadOnlyRootFS should be true")
				}
				if config.Network != "none" {
					return fmt.Errorf("Network should be none, got %s", config.Network)
				}
				if len(config.Security.AddCapabilities) != 0 {
					return fmt.Errorf("Should have no capabilities")
				}
				return nil
			},
		},
		{
			name:    "moderate profile",
			profile: SecurityProfileModerate,
			validate: func(config *ContainerConfig) error {
				if !config.Security.NoNewPrivileges {
					return fmt.Errorf("NoNewPrivileges should be true")
				}
				if len(config.Security.AddCapabilities) == 0 {
					return fmt.Errorf("Should have some capabilities")
				}
				// Should have CHOWN capability
				hasChown := false
				for _, cap := range config.Security.AddCapabilities {
					if cap == "CHOWN" {
						hasChown = true
						break
					}
				}
				if !hasChown {
					return fmt.Errorf("Should have CHOWN capability")
				}
				return nil
			},
		},
		{
			name:    "minimal profile",
			profile: SecurityProfileMinimal,
			validate: func(config *ContainerConfig) error {
				if !config.Security.NoNewPrivileges {
					return fmt.Errorf("NoNewPrivileges should be true")
				}
				if config.Security.ReadOnlyRootFS {
					return fmt.Errorf("ReadOnlyRootFS should be false for minimal")
				}
				if config.Network != "bridge" {
					return fmt.Errorf("Network should be bridge, got %s", config.Network)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ContainerConfig{
				Image:    "test:latest",
				Security: &SecurityConfig{},
			}

			err := sm.ApplySecurityProfile(config, tt.profile)
			if err != nil {
				t.Fatalf("ApplySecurityProfile() failed: %v", err)
			}

			if err := tt.validate(config); err != nil {
				t.Errorf("Profile validation failed: %v", err)
			}
		})
	}
}

func TestSecurityManager_ValidateNetworkAccess(t *testing.T) {
	tmpDir := t.TempDir()
	sm, err := NewSecurityManager(filepath.Join(tmpDir, "audit.log"), false)
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}
	defer sm.Close()

	tests := []struct {
		name    string
		network string
		policy  *NetworkPolicy
		wantErr bool
	}{
		{
			name:    "none network always allowed",
			network: "none",
			policy:  &NetworkPolicy{},
			wantErr: false,
		},
		{
			name:    "bridge network blocked by default",
			network: "bridge",
			policy:  &NetworkPolicy{},
			wantErr: true,
		},
		{
			name:    "bridge network allowed with localhost",
			network: "bridge",
			policy: &NetworkPolicy{
				AllowLocalhost: true,
			},
			wantErr: false,
		},
		{
			name:    "host network blocked by default",
			network: "host",
			policy:  &NetworkPolicy{},
			wantErr: true,
		},
		{
			name:    "custom network with allowed hosts",
			network: "custom",
			policy: &NetworkPolicy{
				AllowedHosts: []string{"github.com", "registry.docker.io"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm.SetNetworkPolicy(tt.policy)

			config := &ContainerConfig{
				Security: &SecurityConfig{},
			}

			err := sm.ValidateNetworkAccess(tt.network, config)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNetworkAccess() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify network isolation flag
			if tt.network == "none" && !config.Security.NetworkIsolation {
				t.Error("NetworkIsolation should be true for none network")
			}
			if tt.network != "none" && config.Security.NetworkIsolation {
				t.Error("NetworkIsolation should be false for non-none network")
			}
		})
	}
}

func TestGenerateSecureContainerName(t *testing.T) {
	names := make(map[string]bool)

	// Generate multiple names to ensure uniqueness
	for i := 0; i < 10; i++ {
		name, err := GenerateSecureContainerName("test")
		if err != nil {
			t.Fatalf("GenerateSecureContainerName() failed: %v", err)
		}

		// Check format
		if !strings.HasPrefix(name, "test-") {
			t.Errorf("Name should start with prefix, got %s", name)
		}

		// Check uniqueness
		if names[name] {
			t.Errorf("Generated duplicate name: %s", name)
		}
		names[name] = true

		// Verify contains hex and timestamp
		parts := strings.Split(name, "-")
		if len(parts) != 3 {
			t.Errorf("Name should have 3 parts, got %d", len(parts))
		}
	}
}

func TestSecurityAuditor_LogRotation(t *testing.T) {
	tmpDir := t.TempDir()
	auditLog := filepath.Join(tmpDir, "audit.log")

	// Create auditor with small rotation size for testing
	auditor, err := NewSecurityAuditor(auditLog, false)
	if err != nil {
		t.Fatalf("NewSecurityAuditor() failed: %v", err)
	}

	// Set small rotation size
	auditor.rotateSize = 100 // 100 bytes

	defer auditor.writer.(io.Closer).Close()

	// Log events until rotation occurs
	for i := 0; i < 10; i++ {
		event := AuditEvent{
			Timestamp: time.Now(),
			EventType: "test_rotation",
			Action:    fmt.Sprintf("action-%d", i),
			Resource:  "test-resource",
			Result:    "success",
		}

		if err := auditor.LogEvent(event); err != nil {
			t.Fatalf("LogEvent() failed: %v", err)
		}
	}

	// Check that rotation occurred
	rotatedFile := auditLog + ".1"
	if _, err := os.Stat(rotatedFile); os.IsNotExist(err) {
		t.Error("Rotated log file should exist")
	}

	// Current log file should still exist
	if _, err := os.Stat(auditLog); os.IsNotExist(err) {
		t.Error("Current log file should exist")
	}
}

func TestSecurityManager_AuditContainerExecution(t *testing.T) {
	tmpDir := t.TempDir()
	auditLog := filepath.Join(tmpDir, "audit.log")

	sm, err := NewSecurityManager(auditLog, false)
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}
	defer sm.Close()

	// Create context with values
	ctx := context.WithValue(context.Background(), contextKeyRunID, "test-run-123")
	ctx = context.WithValue(ctx, contextKeyStepID, "test-step-456")

	config := &ContainerConfig{
		Image: "test:latest",
	}

	result := &ContainerResult{
		ExitCode:  0,
		StartTime: time.Now().Add(-5 * time.Second),
		EndTime:   time.Now(),
	}

	// Audit the execution
	sm.AuditContainerExecution(ctx, config, result)

	// Read audit log
	content, err := os.ReadFile(auditLog)
	if err != nil {
		t.Fatalf("Failed to read audit log: %v", err)
	}

	logStr := string(content)
	if !strings.Contains(logStr, `"type":"container_execution"`) {
		t.Error("Audit log should contain container execution event")
	}
	if !strings.Contains(logStr, `"run_id":"test-run-123"`) {
		t.Error("Audit log should contain run ID from context")
	}
	if !strings.Contains(logStr, `"step_id":"test-step-456"`) {
		t.Error("Audit log should contain step ID from context")
	}
}

func TestSecurityManager_SettersAndClose(t *testing.T) {
	tmpDir := t.TempDir()
	sm, err := NewSecurityManager(filepath.Join(tmpDir, "audit.log"), false)
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}

	// Test SetVolumeRestrictions
	newRestrictions := &VolumeRestriction{
		AllowedPaths: []string{"/custom"},
		MaxVolumes:   10,
	}
	sm.SetVolumeRestrictions(newRestrictions)

	if sm.volumeRestrictions.MaxVolumes != 10 {
		t.Error("Volume restrictions not updated")
	}

	// Test SetNetworkPolicy
	newPolicy := &NetworkPolicy{
		AllowedHosts: []string{"example.com"},
	}
	sm.SetNetworkPolicy(newPolicy)

	if len(sm.networkPolicy.AllowedHosts) != 1 {
		t.Error("Network policy not updated")
	}

	// Test SetSeccompProfile
	sm.SetSeccompProfile("/path/to/profile.json")

	if sm.seccompProfile != "/path/to/profile.json" {
		t.Error("Seccomp profile not updated")
	}

	// Test Close
	if err := sm.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}
