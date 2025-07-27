package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// contextKey represents typed context keys for safe context value passing.
type contextKey string

const (
	contextKeyRunID  contextKey = "run_id"
	contextKeyStepID contextKey = "step_id"
)

// SecurityAuditor handles security audit logging and monitoring.
type SecurityAuditor struct {
	logFile     string
	writer      io.Writer
	mu          sync.Mutex
	debug       bool
	rotateSize  int64
	maxLogFiles int
	currentSize int64
}

// AuditEvent represents a security-relevant event.
type AuditEvent struct {
	Timestamp time.Time
	EventType string
	RunID     string
	StepID    string
	User      string
	Action    string
	Resource  string
	Result    string
	Details   map[string]string
}

// SecurityProfile defines container security profiles.
type SecurityProfile string

const (
	SecurityProfileStrict   SecurityProfile = "strict"   // Maximum security restrictions
	SecurityProfileModerate SecurityProfile = "moderate" // Balanced security
	SecurityProfileMinimal  SecurityProfile = "minimal"  // Minimal restrictions (testing only)
)

// NetworkPolicy defines network access policies.
type NetworkPolicy struct {
	AllowedHosts   []string // Specific hosts that can be accessed
	AllowedPorts   []int    // Specific ports that can be accessed
	BlockedHosts   []string // Hosts that must be blocked
	DNSPolicy      string   // DNS resolution policy
	AllowLocalhost bool     // Allow localhost connections
}

// VolumeRestriction defines volume mount restrictions.
type VolumeRestriction struct {
	AllowedPaths     []string // Paths that can be mounted
	BlockedPaths     []string // Paths that must not be mounted
	ReadOnlyPaths    []string // Paths that must be read-only
	MaxVolumes       int      // Maximum number of volumes
	AllowTempVolumes bool     // Allow temporary volumes
}

// SecurityManager handles advanced security features.
type SecurityManager struct {
	auditor            *SecurityAuditor
	volumeRestrictions *VolumeRestriction
	networkPolicy      *NetworkPolicy
	seccompProfile     string
	enableAudit        bool
	debug              bool
	mu                 sync.RWMutex
}

// NewSecurityManager creates a new security manager.
func NewSecurityManager(auditLogPath string, debug bool) (*SecurityManager, error) {
	auditor, err := NewSecurityAuditor(auditLogPath, debug)
	if err != nil {
		return nil, fmt.Errorf("failed to create security auditor: %w", err)
	}

	return &SecurityManager{
		auditor:     auditor,
		enableAudit: true,
		debug:       debug,
		volumeRestrictions: &VolumeRestriction{
			AllowedPaths: []string{"/workspace", "/tmp"},
			BlockedPaths: []string{
				"/etc", "/sys", "/proc", "/dev",
				"/root", "/home", "/var/run/docker.sock",
			},
			ReadOnlyPaths:    []string{"/usr", "/bin", "/sbin", "/lib"},
			MaxVolumes:       5,
			AllowTempVolumes: true,
		},
		networkPolicy: &NetworkPolicy{
			AllowedHosts:   []string{},
			BlockedHosts:   []string{"metadata.google.internal", "169.254.169.254"},
			DNSPolicy:      "none",
			AllowLocalhost: false,
		},
	}, nil
}

// NewSecurityAuditor creates a new security auditor.
func NewSecurityAuditor(logPath string, debug bool) (*SecurityAuditor, error) {
	// Ensure log directory exists
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create audit log directory: %w", err)
	}

	// Open log file with secure permissions
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log file: %w", err)
	}

	return &SecurityAuditor{
		logFile:     logPath,
		writer:      file,
		debug:       debug,
		rotateSize:  10 * 1024 * 1024, // 10MB
		maxLogFiles: 5,
	}, nil
}

// LogEvent logs a security audit event.
func (sa *SecurityAuditor) LogEvent(event AuditEvent) error {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	// Format event as JSON for structured logging
	eventStr := fmt.Sprintf(`{"timestamp":"%s","type":"%s","run_id":"%s","step_id":"%s","user":"%s","action":"%s","resource":"%s","result":"%s"`,
		event.Timestamp.Format(time.RFC3339),
		event.EventType,
		event.RunID,
		event.StepID,
		event.User,
		event.Action,
		event.Resource,
		event.Result,
	)

	// Add details if present
	if len(event.Details) > 0 {
		eventStr += `,"details":{`
		first := true
		for k, v := range event.Details {
			if !first {
				eventStr += ","
			}
			eventStr += fmt.Sprintf(`"%s":"%s"`, k, v)
			first = false
		}
		eventStr += "}"
	}
	eventStr += "}\n"

	// Write to log
	n, err := sa.writer.Write([]byte(eventStr))
	if err != nil {
		return fmt.Errorf("failed to write audit event: %w", err)
	}

	sa.currentSize += int64(n)

	// Check if rotation is needed
	if sa.currentSize > sa.rotateSize {
		if err := sa.rotateLog(); err != nil {
			return fmt.Errorf("failed to rotate audit log: %w", err)
		}
	}

	return nil
}

// rotateLog rotates the audit log file.
func (sa *SecurityAuditor) rotateLog() error {
	// Close current file
	if closer, ok := sa.writer.(io.Closer); ok {
		closer.Close()
	}

	// Rotate existing files
	for i := sa.maxLogFiles - 1; i > 0; i-- {
		oldPath := fmt.Sprintf("%s.%d", sa.logFile, i)
		newPath := fmt.Sprintf("%s.%d", sa.logFile, i+1)
		os.Rename(oldPath, newPath)
	}

	// Rename current file
	os.Rename(sa.logFile, sa.logFile+".1")

	// Create new file
	file, err := os.OpenFile(sa.logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	sa.writer = file
	sa.currentSize = 0
	return nil
}

// ValidateVolumeMounts validates volume mounts against security restrictions.
func (sm *SecurityManager) ValidateVolumeMounts(volumes []VolumeMount) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(volumes) > sm.volumeRestrictions.MaxVolumes {
		return fmt.Errorf("too many volume mounts: %d (max: %d)",
			len(volumes), sm.volumeRestrictions.MaxVolumes)
	}

	for _, vol := range volumes {
		// Check if path is blocked
		for _, blocked := range sm.volumeRestrictions.BlockedPaths {
			if strings.HasPrefix(vol.Source, blocked) {
				return fmt.Errorf("volume mount blocked: %s is in restricted path %s",
					vol.Source, blocked)
			}
		}

		// Check if path is allowed
		allowed := false
		for _, allowedPath := range sm.volumeRestrictions.AllowedPaths {
			if strings.HasPrefix(vol.Source, allowedPath) {
				allowed = true
				break
			}
		}

		if !allowed && !sm.volumeRestrictions.AllowTempVolumes {
			return fmt.Errorf("volume mount not allowed: %s is not in allowed paths", vol.Source)
		}

		// Check read-only requirements
		for _, roPath := range sm.volumeRestrictions.ReadOnlyPaths {
			if strings.HasPrefix(vol.Source, roPath) && !vol.ReadOnly {
				return fmt.Errorf("volume mount must be read-only: %s", vol.Source)
			}
		}
	}

	return nil
}

// ApplySecurityProfile applies a security profile to container configuration.
func (sm *SecurityManager) ApplySecurityProfile(config *ContainerConfig, profile SecurityProfile) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	switch profile {
	case SecurityProfileStrict:
		// Maximum security restrictions
		config.Security.NoNewPrivileges = true
		config.Security.ReadOnlyRootFS = true
		config.Security.DropCapabilities = []string{"ALL"}
		config.Security.AddCapabilities = []string{} // No capabilities
		config.Security.SeccompProfile = "runtime/default"
		config.Security.NetworkIsolation = true
		config.Network = "none"

	case SecurityProfileModerate:
		// Balanced security
		config.Security.NoNewPrivileges = true
		config.Security.ReadOnlyRootFS = true
		config.Security.DropCapabilities = []string{"ALL"}
		config.Security.AddCapabilities = []string{"CHOWN", "SETUID", "SETGID"}
		config.Security.SeccompProfile = "runtime/default"
		if config.Network == "" {
			config.Network = "none"
		}

	case SecurityProfileMinimal:
		// Minimal restrictions (testing only)
		config.Security.NoNewPrivileges = true
		config.Security.DropCapabilities = []string{"NET_RAW", "SYS_ADMIN"}
		if config.Network == "" {
			config.Network = "bridge"
		}

	default:
		return fmt.Errorf("unknown security profile: %s", profile)
	}

	// Apply additional security settings
	if sm.seccompProfile != "" {
		config.Security.SeccompProfile = sm.seccompProfile
	}

	// Log security profile application
	if sm.enableAudit {
		event := AuditEvent{
			Timestamp: time.Now(),
			EventType: "security_profile",
			Action:    "apply",
			Resource:  string(profile),
			Result:    "success",
			Details: map[string]string{
				"image": config.Image,
			},
		}
		sm.auditor.LogEvent(event)
	}

	return nil
}

// GenerateSecureContainerName generates a cryptographically secure container name.
func GenerateSecureContainerName(prefix string) (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	return fmt.Sprintf("%s-%s-%d", prefix, hex.EncodeToString(bytes), time.Now().Unix()), nil
}

// ValidateNetworkAccess validates network configuration against policy.
func (sm *SecurityManager) ValidateNetworkAccess(network string, config *ContainerConfig) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Check if network access is allowed at all
	if network != "none" && !sm.isNetworkAllowed() {
		return fmt.Errorf("network access not allowed by security policy")
	}

	// Apply network isolation for security
	if config.Security != nil {
		config.Security.NetworkIsolation = (network == "none")
	}

	return nil
}

// isNetworkAllowed checks if network access is permitted.
func (sm *SecurityManager) isNetworkAllowed() bool {
	// In strict mode, no network access is allowed
	return len(sm.networkPolicy.AllowedHosts) > 0 || sm.networkPolicy.AllowLocalhost
}

// AuditContainerExecution logs container execution for audit.
func (sm *SecurityManager) AuditContainerExecution(ctx context.Context, config *ContainerConfig, result *ContainerResult) {
	if !sm.enableAudit {
		return
	}

	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "container_execution",
		Action:    "execute",
		Resource:  config.Image,
		Result:    "success",
		Details: map[string]string{
			"exit_code":   fmt.Sprintf("%d", result.ExitCode),
			"duration_ms": fmt.Sprintf("%d", result.EndTime.Sub(result.StartTime).Milliseconds()),
		},
	}

	if result.ExitCode != 0 {
		event.Result = "failure"
	}

	// Extract run context from context
	if runID, ok := ctx.Value(contextKeyRunID).(string); ok {
		event.RunID = runID
	}
	if stepID, ok := ctx.Value(contextKeyStepID).(string); ok {
		event.StepID = stepID
	}

	sm.auditor.LogEvent(event)
}

// SetVolumeRestrictions updates volume mount restrictions.
func (sm *SecurityManager) SetVolumeRestrictions(restrictions *VolumeRestriction) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.volumeRestrictions = restrictions
}

// SetNetworkPolicy updates network access policy.
func (sm *SecurityManager) SetNetworkPolicy(policy *NetworkPolicy) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.networkPolicy = policy
}

// SetSeccompProfile sets the seccomp profile path.
func (sm *SecurityManager) SetSeccompProfile(profile string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.seccompProfile = profile
}

// Close closes the security manager and its resources.
func (sm *SecurityManager) Close() error {
	if sm.auditor != nil {
		if closer, ok := sm.auditor.writer.(io.Closer); ok {
			return closer.Close()
		}
	}
	return nil
}
