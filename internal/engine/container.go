package engine

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dangazineu/tako/internal/config"
)

// ContainerRuntime represents the detected container runtime.
type ContainerRuntime string

const (
	RuntimeDocker ContainerRuntime = "docker"
	RuntimePodman ContainerRuntime = "podman"
	RuntimeNone   ContainerRuntime = "none"
)

// ContainerConfig holds configuration for container execution.
type ContainerConfig struct {
	Image        string
	Command      []string
	Entrypoint   []string
	WorkDir      string
	Env          map[string]string
	Volumes      []VolumeMount
	Network      string
	Capabilities []string
	Resources    *ResourceLimits
	Security     *SecurityConfig
}

// VolumeMount represents a volume mount configuration.
type VolumeMount struct {
	Source      string
	Destination string
	ReadOnly    bool
}

// ResourceLimits holds container resource constraints.
type ResourceLimits struct {
	CPULimit    string
	MemoryLimit string
	DiskLimit   string
}

// SecurityConfig holds container security configuration.
type SecurityConfig struct {
	RunAsUser        int
	ReadOnlyRootFS   bool
	NoNewPrivileges  bool
	DropCapabilities []string
	AddCapabilities  []string
	SeccompProfile   string
	NetworkIsolation bool
}

// ContainerManager handles container operations with security hardening.
type ContainerManager struct {
	runtime         ContainerRuntime
	securityManager *SecurityManager
	registryManager *RegistryManager
	defaultProfile  SecurityProfile
	debug           bool
}

// NewContainerManager creates a new container manager with runtime auto-detection.
func NewContainerManager(debug bool) (*ContainerManager, error) {
	runtime, err := detectContainerRuntime()
	if err != nil {
		return nil, fmt.Errorf("failed to detect container runtime: %w", err)
	}

	if runtime == RuntimeNone {
		return nil, fmt.Errorf("no supported container runtime found (docker or podman required)")
	}

	return &ContainerManager{
		runtime:        runtime,
		defaultProfile: SecurityProfileModerate,
		debug:          debug,
	}, nil
}

// WithSecurityManager sets the security manager.
func (cm *ContainerManager) WithSecurityManager(sm *SecurityManager) *ContainerManager {
	cm.securityManager = sm
	return cm
}

// WithRegistryManager sets the registry manager.
func (cm *ContainerManager) WithRegistryManager(rm *RegistryManager) *ContainerManager {
	cm.registryManager = rm
	return cm
}

// detectContainerRuntime auto-detects available container runtime.
// Returns error for interface consistency (currently always nil).
func detectContainerRuntime() (ContainerRuntime, error) {
	// Check for Docker first
	if _, err := exec.LookPath("docker"); err == nil {
		// Verify Docker is actually running
		cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
		if err := cmd.Run(); err == nil {
			return RuntimeDocker, nil
		}
	}

	// Check for Podman
	if _, err := exec.LookPath("podman"); err == nil {
		// Verify Podman is accessible
		cmd := exec.Command("podman", "version", "--format", "{{.Server.Version}}")
		if err := cmd.Run(); err == nil {
			return RuntimePodman, nil
		}
		// Podman might work in rootless mode without server
		cmd = exec.Command("podman", "info", "--format", "{{.Version.Version}}")
		if err := cmd.Run(); err == nil {
			return RuntimePodman, nil
		}
	}

	return RuntimeNone, nil
}

// ValidateContainerConfig validates container configuration early.
func (cm *ContainerManager) ValidateContainerConfig(step config.WorkflowStep) error {
	if step.Image == "" {
		return fmt.Errorf("container image is required for containerized steps")
	}

	// Validate image name format
	if !isValidImageName(step.Image) {
		return fmt.Errorf("invalid container image name: %s", step.Image)
	}

	// Validate network configuration
	if step.Network != "" && !isValidNetworkName(step.Network) {
		return fmt.Errorf("invalid network name: %s", step.Network)
	}

	// Validate capabilities
	for _, cap := range step.Capabilities {
		if !isValidCapability(cap) {
			return fmt.Errorf("invalid capability: %s", cap)
		}
	}

	return nil
}

// isValidImageName checks if the image name follows valid format.
func isValidImageName(image string) bool {
	if image == "" {
		return false
	}

	// Basic validation for image name format
	// Format: [registry/]namespace/name[:tag][@digest]
	// Should not contain consecutive dots or invalid characters
	if strings.Contains(image, "..") || strings.Contains(image, "!") {
		return false
	}

	// More strict regex for image names
	imageRegex := regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?([:\d]+)?/)?[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?(/[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?)*(:[\w.-]+)?(@sha256:[a-f0-9]{64})?$`)
	return imageRegex.MatchString(image)
}

// isValidNetworkName checks if the network name is valid.
func isValidNetworkName(network string) bool {
	// Allow standard network names and none/host
	validNetworks := []string{"none", "host", "bridge", "default"}
	for _, valid := range validNetworks {
		if network == valid {
			return true
		}
	}
	// Allow custom network names (basic validation - must start with letter)
	networkRegex := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]*$`)
	return networkRegex.MatchString(network)
}

// isValidCapability checks if the capability name is valid.
func isValidCapability(capability string) bool {
	// Remove optional CAP_ prefix
	cap := strings.TrimPrefix(strings.ToUpper(capability), "CAP_")

	// List of known Linux capabilities
	validCapabilities := []string{
		"AUDIT_CONTROL", "AUDIT_READ", "AUDIT_WRITE", "BLOCK_SUSPEND", "CHOWN",
		"DAC_OVERRIDE", "DAC_READ_SEARCH", "FOWNER", "FSETID", "IPC_LOCK",
		"IPC_OWNER", "KILL", "LEASE", "LINUX_IMMUTABLE", "MAC_ADMIN",
		"MAC_OVERRIDE", "MKNOD", "NET_ADMIN", "NET_BIND_SERVICE", "NET_BROADCAST",
		"NET_RAW", "SETGID", "SETFCAP", "SETPCAP", "SETUID", "SYS_ADMIN",
		"SYS_BOOT", "SYS_CHROOT", "SYS_MODULE", "SYS_NICE", "SYS_PACCT",
		"SYS_PTRACE", "SYS_RAWIO", "SYS_RESOURCE", "SYS_TIME", "SYS_TTY_CONFIG",
		"SYSLOG", "WAKE_ALARM",
	}

	for _, valid := range validCapabilities {
		if cap == valid {
			return true
		}
	}
	return false
}

// validateVolumePath validates a volume path for security.
func validateVolumePath(path string) error {
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal detected in volume path: %s", path)
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("relative paths not allowed for volume mounts: %s", path)
	}
	return nil
}

// BuildContainerConfig creates container configuration from workflow step.
func (cm *ContainerManager) BuildContainerConfig(step config.WorkflowStep, workDir string, env map[string]string, resources *config.Resources) (*ContainerConfig, error) {
	// Validate first
	if err := cm.ValidateContainerConfig(step); err != nil {
		return nil, err
	}

	config := &ContainerConfig{
		Image:   step.Image,
		WorkDir: "/workspace",
		Env:     make(map[string]string),
	}

	// Set command/entrypoint
	if step.Run != "" {
		config.Command = []string{"sh", "-c", step.Run}
	}

	// Copy environment variables
	for k, v := range env {
		config.Env[k] = v
	}

	// Add step-specific environment variables
	for k, v := range step.Env {
		config.Env[k] = v
	}

	// Add Tako-specific environment variables
	config.Env["TAKO_CONTAINER"] = "true"
	config.Env["TAKO_RUNTIME"] = string(cm.runtime)

	// Configure volumes (workspace volume - read-only by default for security)
	config.Volumes = []VolumeMount{
		{
			Source:      workDir,
			Destination: "/workspace",
			ReadOnly:    false, // Allow writes to workspace
		},
	}

	// Add any additional volumes from step configuration
	for _, vol := range step.Volumes {
		// Validate volume paths for security
		if err := validateVolumePath(vol.Source); err != nil {
			return nil, fmt.Errorf("invalid volume source path: %w", err)
		}
		if err := validateVolumePath(vol.Destination); err != nil {
			return nil, fmt.Errorf("invalid volume destination path: %w", err)
		}

		config.Volumes = append(config.Volumes, VolumeMount{
			Source:      vol.Source,
			Destination: vol.Destination,
			ReadOnly:    vol.ReadOnly,
		})
	}

	// Validate volumes with security manager if available
	if cm.securityManager != nil {
		if err := cm.securityManager.ValidateVolumeMounts(config.Volumes); err != nil {
			return nil, fmt.Errorf("volume validation failed: %w", err)
		}
	}

	// Configure network (isolated by default for security)
	config.Network = "none" // Default: no network access
	if step.Network != "" {
		config.Network = step.Network
	}

	// Configure security settings with secure defaults
	config.Security = &SecurityConfig{
		RunAsUser:        1001,            // Non-root user
		ReadOnlyRootFS:   true,            // Read-only root filesystem
		NoNewPrivileges:  true,            // Prevent privilege escalation
		DropCapabilities: []string{"ALL"}, // Drop all capabilities by default
		NetworkIsolation: config.Network == "none",
	}

	// Allow specific capabilities if requested
	if len(step.Capabilities) > 0 {
		config.Security.AddCapabilities = step.Capabilities
	}

	// Apply security profile if security manager is available
	if cm.securityManager != nil {
		// Use the profile specified in step or default profile
		profile := cm.defaultProfile
		if step.SecurityProfile != "" {
			profile = SecurityProfile(step.SecurityProfile)
		}
		if err := cm.securityManager.ApplySecurityProfile(config, profile); err != nil {
			return nil, fmt.Errorf("failed to apply security profile: %w", err)
		}

		// Validate network access
		if err := cm.securityManager.ValidateNetworkAccess(config.Network, config); err != nil {
			return nil, fmt.Errorf("network access validation failed: %w", err)
		}
	}

	// Configure resource limits if provided
	if resources != nil {
		config.Resources = &ResourceLimits{}

		if resources.CPULimit != "" {
			config.Resources.CPULimit = resources.CPULimit
		}
		if resources.MemLimit != "" {
			config.Resources.MemoryLimit = resources.MemLimit
		}
		if resources.DiskLimit != "" {
			config.Resources.DiskLimit = resources.DiskLimit
		}
	}

	return config, nil
}

// RunContainer executes a container with the given configuration.
func (cm *ContainerManager) RunContainer(ctx context.Context, containerConfig *ContainerConfig, stepID string) (*ContainerResult, error) {
	startTime := time.Now()

	// Generate secure container name if security manager is available
	var containerName string
	var err error
	if cm.securityManager != nil {
		containerName, err = GenerateSecureContainerName(fmt.Sprintf("tako-%s", stepID))
		if err != nil {
			return nil, fmt.Errorf("failed to generate secure container name: %w", err)
		}
	} else {
		// Fallback to simple name generation
		containerName = fmt.Sprintf("tako-%s-%d", stepID, startTime.Unix())
	}

	// Build container run command
	args, err := cm.buildRunCommand(containerName, containerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build container command: %w", err)
	}

	if cm.debug {
		fmt.Printf("Container command: %s %s\n", cm.runtime, strings.Join(args, " "))
	}

	// Create command with context for cancellation
	cmd := exec.CommandContext(ctx, string(cm.runtime), args...)

	// Set up output capture
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute container
	err = cmd.Run()

	result := &ContainerResult{
		ContainerName: containerName,
		ExitCode:      0,
		Stdout:        stdout.String(),
		Stderr:        stderr.String(),
		StartTime:     startTime,
		EndTime:       time.Now(),
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		} else {
			return nil, fmt.Errorf("failed to run container %s with image %s: %w", containerName, containerConfig.Image, err)
		}
	}

	// Audit container execution if security manager is available
	if cm.securityManager != nil {
		cm.securityManager.AuditContainerExecution(ctx, containerConfig, result)
	}

	// Clean up container if it still exists
	if err := cm.cleanupContainer(containerName); err != nil && cm.debug {
		fmt.Printf("Warning: failed to cleanup container %s: %v\n", containerName, err)
	}

	return result, nil
}

// ContainerResult represents the result of container execution.
type ContainerResult struct {
	ContainerName string
	ExitCode      int
	Stdout        string
	Stderr        string
	StartTime     time.Time
	EndTime       time.Time
}

// buildRunCommand builds the container run command arguments.
// Returns error for future extensibility (currently always nil).
func (cm *ContainerManager) buildRunCommand(containerName string, config *ContainerConfig) ([]string, error) {
	args := []string{"run", "--rm", "--name", containerName}

	// Security hardening
	if config.Security != nil {
		security := config.Security

		// Run as non-root user
		args = append(args, "--user", fmt.Sprintf("%d:%d", security.RunAsUser, security.RunAsUser))

		// Read-only root filesystem
		if security.ReadOnlyRootFS {
			args = append(args, "--read-only")
		}

		// No new privileges
		if security.NoNewPrivileges {
			args = append(args, "--security-opt", "no-new-privileges:true")
		}

		// Drop capabilities
		for _, cap := range security.DropCapabilities {
			args = append(args, "--cap-drop", cap)
		}

		// Add specific capabilities
		for _, cap := range security.AddCapabilities {
			args = append(args, "--cap-add", cap)
		}

		// Apply seccomp profile if specified (skip runtime/default for Docker)
		if security.SeccompProfile != "" && security.SeccompProfile != "runtime/default" {
			args = append(args, "--security-opt", fmt.Sprintf("seccomp=%s", security.SeccompProfile))
		}
	}

	// Network configuration
	args = append(args, "--network", config.Network)

	// Working directory
	if config.WorkDir != "" {
		args = append(args, "--workdir", config.WorkDir)
	}

	// Environment variables
	for key, value := range config.Env {
		args = append(args, "--env", fmt.Sprintf("%s=%s", key, value))
	}

	// Volume mounts
	for _, volume := range config.Volumes {
		mount := fmt.Sprintf("%s:%s", volume.Source, volume.Destination)
		if volume.ReadOnly {
			mount += ":ro"
		}
		args = append(args, "--volume", mount)
	}

	// Resource limits (if supported by runtime)
	if config.Resources != nil {
		if config.Resources.CPULimit != "" {
			args = append(args, "--cpus", config.Resources.CPULimit)
		}
		if config.Resources.MemoryLimit != "" {
			args = append(args, "--memory", config.Resources.MemoryLimit)
		}
	}

	// Container image
	args = append(args, config.Image)

	// Command and arguments
	if len(config.Command) > 0 {
		args = append(args, config.Command...)
	}

	return args, nil
}

// cleanupContainer removes the container if it still exists.
func (cm *ContainerManager) cleanupContainer(containerName string) error {
	// Try to remove the container (in case --rm didn't work)
	cmd := exec.Command(string(cm.runtime), "rm", "-f", containerName)
	return cmd.Run()
}

// PullImage pulls a container image if not already present.
func (cm *ContainerManager) PullImage(ctx context.Context, image string) error {
	// Check cache first if registry manager is available
	if cm.registryManager != nil && cm.registryManager.imageCache != nil {
		_, _, _, tag := ParseImageName(image)
		if _, exists := cm.registryManager.imageCache.GetCachedImage(image, tag); exists {
			if cm.debug {
				fmt.Printf("Using cached image: %s\n", image)
			}
			return nil
		}
	}

	if cm.debug {
		fmt.Printf("Pulling container image: %s\n", image)
	}

	// Build pull command
	args := []string{"pull"}

	// Add authentication if registry manager is available
	if cm.registryManager != nil {
		registry, _, _, _ := ParseImageName(image)
		authStr, err := cm.registryManager.GetAuthString(registry)
		if err == nil && authStr != "" {
			// For Docker, use --auth flag; for Podman use --creds
			switch cm.runtime {
			case RuntimeDocker:
				// Docker doesn't support inline auth, we need to login first
				creds, _ := cm.registryManager.GetCredentials(registry)
				if creds != nil && creds.Username != "" && creds.Password != "" {
					// Login to registry
					loginCmd := exec.CommandContext(ctx, string(cm.runtime), "login",
						"--username", creds.Username,
						"--password-stdin", registry)
					loginCmd.Stdin = strings.NewReader(creds.Password)
					if err := loginCmd.Run(); err != nil && cm.debug {
						fmt.Printf("Warning: failed to login to registry %s: %v\n", registry, err)
					}
				}
			case RuntimePodman:
				// Podman supports inline credentials
				creds, _ := cm.registryManager.GetCredentials(registry)
				if creds != nil && creds.Username != "" && creds.Password != "" {
					args = append(args, "--creds", fmt.Sprintf("%s:%s", creds.Username, creds.Password))
				}
			}
		}
	}

	args = append(args, image)
	cmd := exec.CommandContext(ctx, string(cm.runtime), args...)

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w\nOutput: %s", image, err, string(output))
	}

	if cm.debug {
		fmt.Printf("Successfully pulled image: %s\n", image)
	}

	// Cache the image if registry manager is available
	if cm.registryManager != nil && cm.registryManager.imageCache != nil {
		registry, _, _, tag := ParseImageName(image)
		entry := &ImageCacheEntry{
			Image:    image,
			Registry: registry,
			Tag:      tag,
			PullTime: time.Now(),
			LastUsed: time.Now(),
		}
		cm.registryManager.imageCache.CacheImage(entry)
	}

	return nil
}

// IsContainerStep checks if a workflow step should be executed in a container.
func IsContainerStep(step config.WorkflowStep) bool {
	return step.Image != ""
}
