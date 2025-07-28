package engine

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dangazineu/tako/internal/config"
)

func TestDetectContainerRuntime(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping container runtime detection test in short mode")
	}

	runtime, err := detectContainerRuntime()
	if err != nil {
		t.Fatalf("detectContainerRuntime failed: %v", err)
	}

	// Should detect at least one runtime or none
	validRuntimes := []ContainerRuntime{RuntimeDocker, RuntimePodman, RuntimeNone}
	found := false
	for _, valid := range validRuntimes {
		if runtime == valid {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("detectContainerRuntime returned invalid runtime: %s", runtime)
	}

	// If runtime is not None, the corresponding command should be available
	if runtime == RuntimeDocker {
		if _, err := exec.LookPath("docker"); err != nil {
			t.Errorf("Docker runtime detected but docker command not found")
		}
	}
	if runtime == RuntimePodman {
		if _, err := exec.LookPath("podman"); err != nil {
			t.Errorf("Podman runtime detected but podman command not found")
		}
	}
}

func TestValidateVolumePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid absolute path",
			path:    "/workspace/test",
			wantErr: false,
		},
		{
			name:    "valid absolute path with subdirs",
			path:    "/home/user/project",
			wantErr: false,
		},
		{
			name:    "relative path should fail",
			path:    "workspace/test",
			wantErr: true,
			errMsg:  "relative paths not allowed",
		},
		{
			name:    "path traversal should fail",
			path:    "/workspace/../etc/passwd",
			wantErr: true,
			errMsg:  "path traversal detected",
		},
		{
			name:    "path traversal in middle should fail",
			path:    "/workspace/test/../../../etc",
			wantErr: true,
			errMsg:  "path traversal detected",
		},
		{
			name:    "current directory reference",
			path:    "/workspace/./test",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVolumePath(tt.path)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateVolumePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateVolumePath() error = %v, should contain %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestNewContainerManager(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping container manager test in short mode")
	}

	tests := []struct {
		name    string
		debug   bool
		wantErr bool
	}{
		{
			name:    "create with debug disabled",
			debug:   false,
			wantErr: false,
		},
		{
			name:    "create with debug enabled",
			debug:   true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm, err := NewContainerManager(tt.debug)

			// If no container runtime is available, expect error
			runtime, _ := detectContainerRuntime()
			if runtime == RuntimeNone {
				if err == nil {
					t.Errorf("NewContainerManager() expected error when no runtime available")
				}
				return
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("NewContainerManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if cm.debug != tt.debug {
					t.Errorf("NewContainerManager() debug = %v, want %v", cm.debug, tt.debug)
				}
				if cm.runtime == RuntimeNone {
					t.Errorf("NewContainerManager() runtime should not be None when no error")
				}
			}
		})
	}
}

func TestValidateContainerConfig(t *testing.T) {
	cm := &ContainerManager{runtime: RuntimeDocker}

	tests := []struct {
		name    string
		step    config.WorkflowStep
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid basic configuration",
			step: config.WorkflowStep{
				Image: "alpine:latest",
				Run:   "echo hello",
			},
			wantErr: false,
		},
		{
			name: "missing image",
			step: config.WorkflowStep{
				Run: "echo hello",
			},
			wantErr: true,
			errMsg:  "container image is required",
		},
		{
			name: "invalid image name",
			step: config.WorkflowStep{
				Image: "invalid..image..name",
				Run:   "echo hello",
			},
			wantErr: true,
			errMsg:  "invalid container image name",
		},
		{
			name: "valid network configuration",
			step: config.WorkflowStep{
				Image:   "alpine:latest",
				Network: "bridge",
			},
			wantErr: false,
		},
		{
			name: "invalid network name",
			step: config.WorkflowStep{
				Image:   "alpine:latest",
				Network: "invalid-network-name!",
			},
			wantErr: true,
			errMsg:  "invalid network name",
		},
		{
			name: "valid capabilities",
			step: config.WorkflowStep{
				Image:        "alpine:latest",
				Capabilities: []string{"NET_ADMIN", "CAP_CHOWN"},
			},
			wantErr: false,
		},
		{
			name: "invalid capability",
			step: config.WorkflowStep{
				Image:        "alpine:latest",
				Capabilities: []string{"INVALID_CAP"},
			},
			wantErr: true,
			errMsg:  "invalid capability",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cm.ValidateContainerConfig(tt.step)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContainerConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateContainerConfig() error = %v, should contain %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestIsValidImageName(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  bool
	}{
		{"simple image", "alpine", true},
		{"image with tag", "alpine:latest", true},
		{"image with registry", "docker.io/alpine:latest", true},
		{"image with namespace", "library/alpine", true},
		{"full image path", "registry.example.com:5000/namespace/image:tag", true},
		{"image with digest", "alpine@sha256:abc123def456", false}, // simplified, real digest would be longer
		{"empty image", "", false},
		{"invalid characters", "alpine:latest!", false},
		{"double dots", "alpine..latest", false},
		{"starts with dot", ".alpine", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidImageName(tt.image)
			if got != tt.want {
				t.Errorf("isValidImageName(%q) = %v, want %v", tt.image, got, tt.want)
			}
		})
	}
}

func TestIsValidNetworkName(t *testing.T) {
	tests := []struct {
		name    string
		network string
		want    bool
	}{
		{"none network", "none", true},
		{"host network", "host", true},
		{"bridge network", "bridge", true},
		{"default network", "default", true},
		{"custom network", "my-network", true},
		{"custom network with underscore", "my_network", true},
		{"custom network with dots", "my.network", true},
		{"empty network", "", false},
		{"invalid characters", "my-network!", false},
		{"starts with number", "1network", false},
		{"starts with hyphen", "-network", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidNetworkName(tt.network)
			if got != tt.want {
				t.Errorf("isValidNetworkName(%q) = %v, want %v", tt.network, got, tt.want)
			}
		})
	}
}

func TestIsValidCapability(t *testing.T) {
	tests := []struct {
		name       string
		capability string
		want       bool
	}{
		{"valid capability", "NET_ADMIN", true},
		{"valid capability with prefix", "CAP_NET_ADMIN", true},
		{"valid capability lowercase", "net_admin", true},
		{"chown capability", "CHOWN", true},
		{"sys_admin capability", "SYS_ADMIN", true},
		{"invalid capability", "INVALID_CAP", false},
		{"empty capability", "", false},
		{"random string", "RANDOM_STRING", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidCapability(tt.capability)
			if got != tt.want {
				t.Errorf("isValidCapability(%q) = %v, want %v", tt.capability, got, tt.want)
			}
		})
	}
}

func TestBuildContainerConfig(t *testing.T) {
	cm := &ContainerManager{runtime: RuntimeDocker}

	tests := []struct {
		name    string
		step    config.WorkflowStep
		workDir string
		env     map[string]string
		wantErr bool
	}{
		{
			name: "basic configuration",
			step: config.WorkflowStep{
				Image: "alpine:latest",
				Run:   "echo hello",
			},
			workDir: "/tmp/test",
			env:     map[string]string{"TEST_VAR": "test_value"},
			wantErr: false,
		},
		{
			name: "configuration with custom environment",
			step: config.WorkflowStep{
				Image: "alpine:latest",
				Run:   "echo hello",
				Env:   map[string]string{"CUSTOM_VAR": "custom_value"},
			},
			workDir: "/tmp/test",
			env:     map[string]string{"TEST_VAR": "test_value"},
			wantErr: false,
		},
		{
			name: "configuration with network",
			step: config.WorkflowStep{
				Image:   "alpine:latest",
				Network: "bridge",
			},
			workDir: "/tmp/test",
			env:     map[string]string{},
			wantErr: false,
		},
		{
			name: "configuration with capabilities",
			step: config.WorkflowStep{
				Image:        "alpine:latest",
				Capabilities: []string{"NET_ADMIN"},
			},
			workDir: "/tmp/test",
			env:     map[string]string{},
			wantErr: false,
		},
		{
			name: "invalid configuration",
			step: config.WorkflowStep{
				Run: "echo hello", // missing image
			},
			workDir: "/tmp/test",
			env:     map[string]string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := cm.BuildContainerConfig(tt.step, tt.workDir, tt.env, nil)

			if (err != nil) != tt.wantErr {
				t.Errorf("BuildContainerConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			// Verify basic configuration
			if config.Image != tt.step.Image {
				t.Errorf("BuildContainerConfig() image = %v, want %v", config.Image, tt.step.Image)
			}

			// Verify security defaults
			if config.Security == nil {
				t.Errorf("BuildContainerConfig() security config should not be nil")
			} else {
				if config.Security.RunAsUser != 1001 {
					t.Errorf("BuildContainerConfig() RunAsUser = %v, want 1001", config.Security.RunAsUser)
				}
				if !config.Security.ReadOnlyRootFS {
					t.Errorf("BuildContainerConfig() ReadOnlyRootFS should be true")
				}
				if !config.Security.NoNewPrivileges {
					t.Errorf("BuildContainerConfig() NoNewPrivileges should be true")
				}
			}

			// Verify Tako environment variables are set
			if config.Env["TAKO_CONTAINER"] != "true" {
				t.Errorf("BuildContainerConfig() TAKO_CONTAINER should be 'true'")
			}
			if config.Env["TAKO_RUNTIME"] != string(cm.runtime) {
				t.Errorf("BuildContainerConfig() TAKO_RUNTIME = %v, want %v", config.Env["TAKO_RUNTIME"], cm.runtime)
			}

			// Verify workspace volume mount
			found := false
			for _, volume := range config.Volumes {
				if volume.Destination == "/workspace" {
					found = true
					if volume.Source != tt.workDir {
						t.Errorf("BuildContainerConfig() workspace volume source = %v, want %v", volume.Source, tt.workDir)
					}
					break
				}
			}
			if !found {
				t.Errorf("BuildContainerConfig() workspace volume not found")
			}

			// Verify network isolation by default
			expectedNetwork := "none"
			if tt.step.Network != "" {
				expectedNetwork = tt.step.Network
			}
			if config.Network != expectedNetwork {
				t.Errorf("BuildContainerConfig() network = %v, want %v", config.Network, expectedNetwork)
			}
		})
	}
}

func TestBuildRunCommand(t *testing.T) {
	cm := &ContainerManager{runtime: RuntimeDocker}

	config := &ContainerConfig{
		Image:   "alpine:latest",
		Command: []string{"sh", "-c", "echo hello"},
		WorkDir: "/workspace",
		Env:     map[string]string{"TEST_VAR": "test_value"},
		Volumes: []VolumeMount{
			{Source: "/tmp/test", Destination: "/workspace", ReadOnly: false},
		},
		Network: "none",
		Security: &SecurityConfig{
			RunAsUser:        1001,
			ReadOnlyRootFS:   true,
			NoNewPrivileges:  true,
			DropCapabilities: []string{"ALL"},
		},
	}

	args, err := cm.buildRunCommand("test-container", config)
	if err != nil {
		t.Fatalf("buildRunCommand() failed: %v", err)
	}

	// Convert to string for easier testing
	cmdStr := strings.Join(args, " ")

	// Verify basic flags
	expectedFlags := []string{
		"run", "--rm", "--name", "test-container",
		"--user", "1001:1001",
		"--read-only",
		"--security-opt", "no-new-privileges:true",
		"--cap-drop", "ALL",
		"--network", "none",
		"--workdir", "/workspace",
		"--env", "TEST_VAR=test_value",
		"--volume", "/tmp/test:/workspace",
		"alpine:latest",
		"sh", "-c", "echo hello",
	}

	for _, flag := range expectedFlags {
		if !strings.Contains(cmdStr, flag) {
			t.Errorf("buildRunCommand() missing flag: %s\nFull command: %s", flag, cmdStr)
		}
	}
}

func TestIsContainerStep(t *testing.T) {
	tests := []struct {
		name string
		step config.WorkflowStep
		want bool
	}{
		{
			name: "container step",
			step: config.WorkflowStep{
				Image: "alpine:latest",
				Run:   "echo hello",
			},
			want: true,
		},
		{
			name: "shell step",
			step: config.WorkflowStep{
				Run: "echo hello",
			},
			want: false,
		},
		{
			name: "built-in step",
			step: config.WorkflowStep{
				Uses: "tako/notify@v1",
			},
			want: false,
		},
		{
			name: "empty step",
			step: config.WorkflowStep{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsContainerStep(tt.step)
			if got != tt.want {
				t.Errorf("IsContainerStep() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestContainerManagerIntegration tests container manager with a simple container
// This test requires a container runtime to be available.
func TestContainerManagerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping container integration test in short mode")
	}

	// Skip if no container runtime is available
	runtime, err := detectContainerRuntime()
	if err != nil || runtime == RuntimeNone {
		t.Skip("No container runtime available for integration test")
	}

	cm, err := NewContainerManager(true)
	if err != nil {
		t.Fatalf("NewContainerManager() failed: %v", err)
	}

	step := config.WorkflowStep{
		Image: "alpine:latest",
		Run:   "echo 'Hello from container'",
	}

	config, err := cm.BuildContainerConfig(step, "/tmp", map[string]string{"TEST_VAR": "test_value"}, nil)
	if err != nil {
		t.Fatalf("BuildContainerConfig() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Pull image first
	err = cm.PullImage(ctx, step.Image)
	if err != nil {
		t.Fatalf("PullImage() failed: %v", err)
	}

	// Run container
	result, err := cm.RunContainer(ctx, config, "test-step")
	if err != nil {
		t.Fatalf("RunContainer() failed: %v", err)
	}

	// Verify result
	if result.ExitCode != 0 {
		t.Errorf("RunContainer() exit code = %v, want 0", result.ExitCode)
	}

	if !strings.Contains(result.Stdout, "Hello from container") {
		t.Errorf("RunContainer() stdout does not contain expected output: %s", result.Stdout)
	}

	if result.StartTime.After(result.EndTime) {
		t.Errorf("RunContainer() start time is after end time")
	}
}
