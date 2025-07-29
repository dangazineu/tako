package engine

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dangazineu/tako/internal/config"
)

// ResourceType represents different types of resources.
type ResourceType string

const (
	ResourceTypeCPU    ResourceType = "cpu"
	ResourceTypeMemory ResourceType = "memory"
	ResourceTypeDisk   ResourceType = "disk"
)

// ResourceUnit represents resource measurement units.
type ResourceUnit string

const (
	// UnitCores represents CPU in cores.
	UnitCores ResourceUnit = "cores"
	// UnitMillicores represents CPU in millicores.
	UnitMillicores ResourceUnit = "m"

	// UnitBytes represents memory in bytes.
	UnitBytes     ResourceUnit = "B"
	UnitKilobytes ResourceUnit = "KB"
	UnitMegabytes ResourceUnit = "MB"
	UnitGigabytes ResourceUnit = "GB"
	UnitKibibytes ResourceUnit = "Ki"
	UnitMebibytes ResourceUnit = "Mi"
	UnitGibibytes ResourceUnit = "Gi"

	// Disk units (same as memory).
)

// ResourceLimit represents a resource constraint with parsed values.
type ResourceLimit struct {
	Type         ResourceType
	Value        float64
	Unit         ResourceUnit
	OriginalSpec string
}

// ResourceQuota represents resource limits at different hierarchical levels.
type ResourceQuota struct {
	Global     map[ResourceType]*ResourceLimit // Global system limits
	Repository map[ResourceType]*ResourceLimit // Per-repository limits
	Step       map[ResourceType]*ResourceLimit // Per-step limits
}

// ResourceUsage represents current resource consumption.
type ResourceUsage struct {
	Type        ResourceType
	Used        float64
	Available   float64
	Percentage  float64
	LastUpdated time.Time
}

// ResourceManager handles hierarchical resource management and monitoring.
type ResourceManager struct {
	globalQuota      *ResourceQuota
	repositoryQuotas map[string]*ResourceQuota // keyed by repository name
	stepQuotas       map[string]*ResourceQuota // keyed by step ID

	// Monitoring
	usageHistory     map[ResourceType][]*ResourceUsage
	warningThreshold float64 // Percentage threshold for warnings (default 90%)

	// Configuration
	monitoringInterval time.Duration
	maxHistoryEntries  int

	// Synchronization
	mu             sync.RWMutex
	stopMonitor    chan struct{}
	monitorRunning bool

	// Callbacks
	onWarning func(resourceType ResourceType, usage *ResourceUsage)
	onBreach  func(resourceType ResourceType, usage *ResourceUsage, limit *ResourceLimit)

	debug bool
}

// ResourceManagerConfig configures the resource manager.
type ResourceManagerConfig struct {
	WarningThreshold   float64       // Default 0.9 (90%)
	MonitoringInterval time.Duration // Default 30 seconds
	MaxHistoryEntries  int           // Default 100
	Debug              bool
}

// NewResourceManager creates a new resource manager with hierarchical limits.
func NewResourceManager(config *ResourceManagerConfig) *ResourceManager {
	if config == nil {
		config = &ResourceManagerConfig{
			WarningThreshold:   0.9,
			MonitoringInterval: 30 * time.Second,
			MaxHistoryEntries:  100,
			Debug:              false,
		}
	}

	rm := &ResourceManager{
		globalQuota:        &ResourceQuota{},
		repositoryQuotas:   make(map[string]*ResourceQuota),
		stepQuotas:         make(map[string]*ResourceQuota),
		usageHistory:       make(map[ResourceType][]*ResourceUsage),
		warningThreshold:   config.WarningThreshold,
		monitoringInterval: config.MonitoringInterval,
		maxHistoryEntries:  config.MaxHistoryEntries,
		stopMonitor:        make(chan struct{}),
		debug:              config.Debug,
	}

	// Initialize global quota with system defaults
	rm.initializeGlobalQuota()

	return rm
}

// initializeGlobalQuota sets up default global resource limits based on system capacity.
func (rm *ResourceManager) initializeGlobalQuota() {
	rm.globalQuota.Global = make(map[ResourceType]*ResourceLimit)
	rm.globalQuota.Repository = make(map[ResourceType]*ResourceLimit)
	rm.globalQuota.Step = make(map[ResourceType]*ResourceLimit)

	// Set conservative global defaults based on system resources
	numCPU := float64(runtime.NumCPU())

	// Global limits: Use 80% of system capacity for safety
	rm.globalQuota.Global[ResourceTypeCPU] = &ResourceLimit{
		Type:         ResourceTypeCPU,
		Value:        numCPU * 0.8,
		Unit:         UnitCores,
		OriginalSpec: fmt.Sprintf("%.1f", numCPU*0.8),
	}

	rm.globalQuota.Global[ResourceTypeMemory] = &ResourceLimit{
		Type:         ResourceTypeMemory,
		Value:        4096, // 4GB default global limit
		Unit:         UnitMegabytes,
		OriginalSpec: "4Gi",
	}

	// Repository defaults: Conservative per-repository limits
	rm.globalQuota.Repository[ResourceTypeCPU] = &ResourceLimit{
		Type:         ResourceTypeCPU,
		Value:        2.0,
		Unit:         UnitCores,
		OriginalSpec: "2.0",
	}

	rm.globalQuota.Repository[ResourceTypeMemory] = &ResourceLimit{
		Type:         ResourceTypeMemory,
		Value:        1024, // 1GB default per repository
		Unit:         UnitMegabytes,
		OriginalSpec: "1Gi",
	}

	// Step defaults: Conservative per-step limits
	rm.globalQuota.Step[ResourceTypeCPU] = &ResourceLimit{
		Type:         ResourceTypeCPU,
		Value:        1.0,
		Unit:         UnitCores,
		OriginalSpec: "1.0",
	}

	rm.globalQuota.Step[ResourceTypeMemory] = &ResourceLimit{
		Type:         ResourceTypeMemory,
		Value:        512, // 512MB default per step
		Unit:         UnitMegabytes,
		OriginalSpec: "512Mi",
	}
}

// ParseResourceSpec parses a resource specification string into a ResourceLimit.
func ParseResourceSpec(spec string, resourceType ResourceType) (*ResourceLimit, error) {
	if spec == "" {
		return nil, fmt.Errorf("empty resource specification")
	}

	spec = strings.TrimSpace(spec)

	// Handle CPU specifications
	if resourceType == ResourceTypeCPU {
		return parseCPUSpec(spec)
	}

	// Handle memory/disk specifications
	if resourceType == ResourceTypeMemory || resourceType == ResourceTypeDisk {
		return parseMemorySpec(spec, resourceType)
	}

	return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
}

// parseCPUSpec parses CPU resource specifications.
func parseCPUSpec(spec string) (*ResourceLimit, error) {
	// Handle millicores (e.g., "500m", "1000m")
	if strings.HasSuffix(spec, "m") {
		valueStr := strings.TrimSuffix(spec, "m")
		milliValue, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid CPU millicores value: %s", spec)
		}

		return &ResourceLimit{
			Type:         ResourceTypeCPU,
			Value:        milliValue / 1000.0, // Convert to cores
			Unit:         UnitCores,
			OriginalSpec: spec,
		}, nil
	}

	// Handle cores (e.g., "1", "2.5", "0.5")
	coreValue, err := strconv.ParseFloat(spec, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid CPU cores value: %s", spec)
	}

	return &ResourceLimit{
		Type:         ResourceTypeCPU,
		Value:        coreValue,
		Unit:         UnitCores,
		OriginalSpec: spec,
	}, nil
}

// parseMemorySpec parses memory/disk resource specifications.
func parseMemorySpec(spec string, resourceType ResourceType) (*ResourceLimit, error) {
	// Define unit multipliers (to bytes)
	unitMultipliers := map[string]float64{
		"B":   1,
		"KB":  1000,
		"MB":  1000 * 1000,
		"GB":  1000 * 1000 * 1000,
		"Ki":  1024,
		"Mi":  1024 * 1024,
		"Gi":  1024 * 1024 * 1024,
		"KiB": 1024,
		"MiB": 1024 * 1024,
		"GiB": 1024 * 1024 * 1024,
	}

	// Try to match unit suffixes
	for suffix, multiplier := range unitMultipliers {
		if strings.HasSuffix(spec, suffix) {
			valueStr := strings.TrimSuffix(spec, suffix)
			value, err := strconv.ParseFloat(valueStr, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid %s value: %s", resourceType, spec)
			}

			// Convert to megabytes for internal storage (standardized unit)
			mbValue := (value * multiplier) / (1024 * 1024)

			return &ResourceLimit{
				Type:         resourceType,
				Value:        mbValue,
				Unit:         UnitMegabytes,
				OriginalSpec: spec,
			}, nil
		}
	}

	// If no unit suffix, assume MB
	value, err := strconv.ParseFloat(spec, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s value: %s", resourceType, spec)
	}

	return &ResourceLimit{
		Type:         resourceType,
		Value:        value,
		Unit:         UnitMegabytes,
		OriginalSpec: spec,
	}, nil
}

// SetGlobalQuota sets global resource limits.
func (rm *ResourceManager) SetGlobalQuota(cpuLimit, memoryLimit, diskLimit string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if cpuLimit != "" {
		limit, err := ParseResourceSpec(cpuLimit, ResourceTypeCPU)
		if err != nil {
			return fmt.Errorf("invalid global CPU limit: %w", err)
		}
		rm.globalQuota.Global[ResourceTypeCPU] = limit
	}

	if memoryLimit != "" {
		limit, err := ParseResourceSpec(memoryLimit, ResourceTypeMemory)
		if err != nil {
			return fmt.Errorf("invalid global memory limit: %w", err)
		}
		rm.globalQuota.Global[ResourceTypeMemory] = limit
	}

	if diskLimit != "" {
		limit, err := ParseResourceSpec(diskLimit, ResourceTypeDisk)
		if err != nil {
			return fmt.Errorf("invalid global disk limit: %w", err)
		}
		rm.globalQuota.Global[ResourceTypeDisk] = limit
	}

	return nil
}

// SetRepositoryQuota sets resource limits for a specific repository.
func (rm *ResourceManager) SetRepositoryQuota(repoName string, resources config.Resources) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	quota := &ResourceQuota{
		Global:     make(map[ResourceType]*ResourceLimit),
		Repository: make(map[ResourceType]*ResourceLimit),
		Step:       make(map[ResourceType]*ResourceLimit),
	}

	if resources.CPULimit != "" {
		limit, err := ParseResourceSpec(resources.CPULimit, ResourceTypeCPU)
		if err != nil {
			return fmt.Errorf("invalid repository CPU limit: %w", err)
		}
		quota.Repository[ResourceTypeCPU] = limit
	}

	if resources.MemLimit != "" {
		limit, err := ParseResourceSpec(resources.MemLimit, ResourceTypeMemory)
		if err != nil {
			return fmt.Errorf("invalid repository memory limit: %w", err)
		}
		quota.Repository[ResourceTypeMemory] = limit
	}

	if resources.DiskLimit != "" {
		limit, err := ParseResourceSpec(resources.DiskLimit, ResourceTypeDisk)
		if err != nil {
			return fmt.Errorf("invalid repository disk limit: %w", err)
		}
		quota.Repository[ResourceTypeDisk] = limit
	}

	rm.repositoryQuotas[repoName] = quota
	return nil
}

// ValidateResourceRequest validates if a resource request can be accommodated.
func (rm *ResourceManager) ValidateResourceRequest(repoName, stepID string, cpuRequest, memoryRequest string) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// Parse resource requests
	var cpuLimit, memoryLimit *ResourceLimit
	var err error

	if cpuRequest != "" {
		cpuLimit, err = ParseResourceSpec(cpuRequest, ResourceTypeCPU)
		if err != nil {
			return fmt.Errorf("invalid CPU request: %w", err)
		}
	}

	if memoryRequest != "" {
		memoryLimit, err = ParseResourceSpec(memoryRequest, ResourceTypeMemory)
		if err != nil {
			return fmt.Errorf("invalid memory request: %w", err)
		}
	}

	// Validate against hierarchical limits
	if cpuLimit != nil {
		if err := rm.validateAgainstLimits(ResourceTypeCPU, cpuLimit.Value, repoName, stepID); err != nil {
			return fmt.Errorf("CPU validation failed: %w", err)
		}
	}

	if memoryLimit != nil {
		if err := rm.validateAgainstLimits(ResourceTypeMemory, memoryLimit.Value, repoName, stepID); err != nil {
			return fmt.Errorf("memory validation failed: %w", err)
		}
	}

	return nil
}

// validateAgainstLimits checks if a resource request fits within hierarchical limits.
func (rm *ResourceManager) validateAgainstLimits(resourceType ResourceType, requestedValue float64, repoName, stepID string) error {
	// Determine the effective limit by checking hierarchy from most specific to least specific
	var effectiveLimit *ResourceLimit

	// 1. Check for step-specific limits (highest priority)
	if stepLimit := rm.getStepLimit(resourceType, stepID); stepLimit != nil {
		effectiveLimit = stepLimit
	} else {
		// 2. Check for repository-specific limits
		if repoLimit := rm.getRepositoryLimit(resourceType, repoName); repoLimit != nil {
			effectiveLimit = repoLimit
		} else {
			// 3. Fall back to global limits
			if globalLimit := rm.getGlobalLimit(resourceType); globalLimit != nil {
				effectiveLimit = globalLimit
			}
		}
	}

	// If we found an effective limit, check against it
	if effectiveLimit != nil {
		if requestedValue > effectiveLimit.Value {
			return fmt.Errorf("requested %s %.2f exceeds limit %.2f %s",
				resourceType, requestedValue, effectiveLimit.Value, effectiveLimit.Unit)
		}
	}

	return nil
}

// getStepLimit retrieves the effective step limit for a resource type.
func (rm *ResourceManager) getStepLimit(resourceType ResourceType, stepID string) *ResourceLimit {
	// Check for step-specific quotas first
	if quota, exists := rm.stepQuotas[stepID]; exists {
		if limit, exists := quota.Step[resourceType]; exists {
			return limit
		}
	}

	// For step limits, we don't fall back to global defaults
	// This allows repository-level limits to take precedence
	return nil
}

// getRepositoryLimit retrieves the effective repository limit for a resource type.
func (rm *ResourceManager) getRepositoryLimit(resourceType ResourceType, repoName string) *ResourceLimit {
	if quota, exists := rm.repositoryQuotas[repoName]; exists {
		if limit, exists := quota.Repository[resourceType]; exists {
			return limit
		}
	}

	// Fall back to global default repository limit
	if limit, exists := rm.globalQuota.Repository[resourceType]; exists {
		return limit
	}

	return nil
}

// getGlobalLimit retrieves the global limit for a resource type.
func (rm *ResourceManager) getGlobalLimit(resourceType ResourceType) *ResourceLimit {
	if limit, exists := rm.globalQuota.Global[resourceType]; exists {
		return limit
	}
	return nil
}

// StartMonitoring begins resource usage monitoring.
func (rm *ResourceManager) StartMonitoring(ctx context.Context) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.monitorRunning {
		return fmt.Errorf("monitoring already running")
	}

	rm.monitorRunning = true

	go rm.monitoringLoop(ctx)

	if rm.debug {
		fmt.Println("Resource monitoring started")
	}

	return nil
}

// StopMonitoring stops resource usage monitoring.
func (rm *ResourceManager) StopMonitoring() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.monitorRunning {
		return
	}

	close(rm.stopMonitor)
	rm.monitorRunning = false
	rm.stopMonitor = make(chan struct{}) // Reset for next use

	if rm.debug {
		fmt.Println("Resource monitoring stopped")
	}
}

// monitoringLoop performs periodic resource monitoring.
func (rm *ResourceManager) monitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(rm.monitoringInterval)
	defer ticker.Stop()

	// Capture the stop channel while holding a read lock to avoid race condition
	rm.mu.RLock()
	stopCh := rm.stopMonitor
	rm.mu.RUnlock()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-ticker.C:
			rm.collectResourceUsage()
		}
	}
}

// collectResourceUsage collects current system resource usage.
func (rm *ResourceManager) collectResourceUsage() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	now := time.Now()

	// Collect CPU usage (simplified - in production would use proper system metrics)
	numCPU := float64(runtime.NumCPU())
	cpuUsage := &ResourceUsage{
		Type:        ResourceTypeCPU,
		Used:        numCPU * 0.3, // Simplified simulation
		Available:   numCPU,
		Percentage:  30.0, // Simplified
		LastUpdated: now,
	}

	// Collect memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memUsedMB := float64(memStats.Alloc) / (1024 * 1024)
	memAvailableMB := 8192.0 // Simplified - 8GB assumption

	memoryUsage := &ResourceUsage{
		Type:        ResourceTypeMemory,
		Used:        memUsedMB,
		Available:   memAvailableMB,
		Percentage:  (memUsedMB / memAvailableMB) * 100,
		LastUpdated: now,
	}

	// Store usage history
	rm.addToHistory(ResourceTypeCPU, cpuUsage)
	rm.addToHistory(ResourceTypeMemory, memoryUsage)

	// Check thresholds and trigger warnings/actions
	rm.checkThresholds(cpuUsage)
	rm.checkThresholds(memoryUsage)
}

// addToHistory adds a resource usage entry to the history.
func (rm *ResourceManager) addToHistory(resourceType ResourceType, usage *ResourceUsage) {
	history := rm.usageHistory[resourceType]

	// Add new entry
	history = append(history, usage)

	// Trim history if it exceeds max entries
	if len(history) > rm.maxHistoryEntries {
		history = history[1:]
	}

	rm.usageHistory[resourceType] = history
}

// checkThresholds checks if resource usage exceeds warning thresholds.
func (rm *ResourceManager) checkThresholds(usage *ResourceUsage) {
	// Check warning threshold
	if usage.Percentage >= (rm.warningThreshold * 100) {
		if rm.onWarning != nil {
			rm.onWarning(usage.Type, usage)
		} else if rm.debug {
			fmt.Printf("Warning: %s usage at %.1f%% (threshold: %.1f%%)\n",
				usage.Type, usage.Percentage, rm.warningThreshold*100)
		}
	}

	// Check for resource limit breaches (100% usage)
	if usage.Percentage >= 100.0 {
		limit := rm.getGlobalLimit(usage.Type)
		if rm.onBreach != nil && limit != nil {
			rm.onBreach(usage.Type, usage, limit)
		} else if rm.debug {
			fmt.Printf("Critical: %s usage at %.1f%% - resource limit breached!\n",
				usage.Type, usage.Percentage)
		}
	}
}

// SetWarningCallback sets the callback function for resource warnings.
func (rm *ResourceManager) SetWarningCallback(callback func(ResourceType, *ResourceUsage)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.onWarning = callback
}

// SetBreachCallback sets the callback function for resource limit breaches.
func (rm *ResourceManager) SetBreachCallback(callback func(ResourceType, *ResourceUsage, *ResourceLimit)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.onBreach = callback
}

// GetUsageHistory returns resource usage history for a given resource type.
func (rm *ResourceManager) GetUsageHistory(resourceType ResourceType) []*ResourceUsage {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	history := rm.usageHistory[resourceType]
	// Return a copy to avoid race conditions
	result := make([]*ResourceUsage, len(history))
	copy(result, history)
	return result
}

// GetCurrentUsage returns the most recent resource usage for a given type.
func (rm *ResourceManager) GetCurrentUsage(resourceType ResourceType) *ResourceUsage {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	history := rm.usageHistory[resourceType]
	if len(history) == 0 {
		return nil
	}

	// Return a copy of the most recent entry
	latest := history[len(history)-1]
	return &ResourceUsage{
		Type:        latest.Type,
		Used:        latest.Used,
		Available:   latest.Available,
		Percentage:  latest.Percentage,
		LastUpdated: latest.LastUpdated,
	}
}
