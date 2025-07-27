package engine

import (
	"context"
	"testing"
	"time"

	"github.com/dangazineu/tako/internal/config"
)

func TestParseResourceSpec(t *testing.T) {
	tests := []struct {
		name         string
		spec         string
		resourceType ResourceType
		want         *ResourceLimit
		wantErr      bool
	}{
		// CPU tests
		{
			name:         "CPU cores",
			spec:         "2.5",
			resourceType: ResourceTypeCPU,
			want: &ResourceLimit{
				Type:         ResourceTypeCPU,
				Value:        2.5,
				Unit:         UnitCores,
				OriginalSpec: "2.5",
			},
			wantErr: false,
		},
		{
			name:         "CPU millicores",
			spec:         "500m",
			resourceType: ResourceTypeCPU,
			want: &ResourceLimit{
				Type:         ResourceTypeCPU,
				Value:        0.5,
				Unit:         UnitCores,
				OriginalSpec: "500m",
			},
			wantErr: false,
		},
		{
			name:         "CPU invalid",
			spec:         "invalid",
			resourceType: ResourceTypeCPU,
			want:         nil,
			wantErr:      true,
		},
		// Memory tests
		{
			name:         "Memory gigabytes",
			spec:         "4Gi",
			resourceType: ResourceTypeMemory,
			want: &ResourceLimit{
				Type:         ResourceTypeMemory,
				Value:        4096, // 4GB in MB
				Unit:         UnitMegabytes,
				OriginalSpec: "4Gi",
			},
			wantErr: false,
		},
		{
			name:         "Memory megabytes",
			spec:         "512Mi",
			resourceType: ResourceTypeMemory,
			want: &ResourceLimit{
				Type:         ResourceTypeMemory,
				Value:        512,
				Unit:         UnitMegabytes,
				OriginalSpec: "512Mi",
			},
			wantErr: false,
		},
		{
			name:         "Memory no unit",
			spec:         "1024",
			resourceType: ResourceTypeMemory,
			want: &ResourceLimit{
				Type:         ResourceTypeMemory,
				Value:        1024,
				Unit:         UnitMegabytes,
				OriginalSpec: "1024",
			},
			wantErr: false,
		},
		{
			name:         "Memory invalid",
			spec:         "invalid",
			resourceType: ResourceTypeMemory,
			want:         nil,
			wantErr:      true,
		},
		// Edge cases
		{
			name:         "Empty spec",
			spec:         "",
			resourceType: ResourceTypeCPU,
			want:         nil,
			wantErr:      true,
		},
		{
			name:         "Unsupported type",
			spec:         "100",
			resourceType: ResourceType("unsupported"),
			want:         nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseResourceSpec(tt.spec, tt.resourceType)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResourceSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if got.Type != tt.want.Type {
				t.Errorf("ParseResourceSpec() Type = %v, want %v", got.Type, tt.want.Type)
			}
			if got.Value != tt.want.Value {
				t.Errorf("ParseResourceSpec() Value = %v, want %v", got.Value, tt.want.Value)
			}
			if got.Unit != tt.want.Unit {
				t.Errorf("ParseResourceSpec() Unit = %v, want %v", got.Unit, tt.want.Unit)
			}
			if got.OriginalSpec != tt.want.OriginalSpec {
				t.Errorf("ParseResourceSpec() OriginalSpec = %v, want %v", got.OriginalSpec, tt.want.OriginalSpec)
			}
		})
	}
}

func TestNewResourceManager(t *testing.T) {
	tests := []struct {
		name   string
		config *ResourceManagerConfig
		want   *ResourceManagerConfig
	}{
		{
			name:   "default config",
			config: nil,
			want: &ResourceManagerConfig{
				WarningThreshold:   0.9,
				MonitoringInterval: 30 * time.Second,
				MaxHistoryEntries:  100,
				Debug:              false,
			},
		},
		{
			name: "custom config",
			config: &ResourceManagerConfig{
				WarningThreshold:   0.8,
				MonitoringInterval: 10 * time.Second,
				MaxHistoryEntries:  50,
				Debug:              true,
			},
			want: &ResourceManagerConfig{
				WarningThreshold:   0.8,
				MonitoringInterval: 10 * time.Second,
				MaxHistoryEntries:  50,
				Debug:              true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := NewResourceManager(tt.config)

			if rm.warningThreshold != tt.want.WarningThreshold {
				t.Errorf("NewResourceManager() warningThreshold = %v, want %v",
					rm.warningThreshold, tt.want.WarningThreshold)
			}
			if rm.monitoringInterval != tt.want.MonitoringInterval {
				t.Errorf("NewResourceManager() monitoringInterval = %v, want %v",
					rm.monitoringInterval, tt.want.MonitoringInterval)
			}
			if rm.maxHistoryEntries != tt.want.MaxHistoryEntries {
				t.Errorf("NewResourceManager() maxHistoryEntries = %v, want %v",
					rm.maxHistoryEntries, tt.want.MaxHistoryEntries)
			}
			if rm.debug != tt.want.Debug {
				t.Errorf("NewResourceManager() debug = %v, want %v", rm.debug, tt.want.Debug)
			}

			// Verify global quota initialization
			if rm.globalQuota.Global[ResourceTypeCPU] == nil {
				t.Error("NewResourceManager() global CPU quota not initialized")
			}
			if rm.globalQuota.Global[ResourceTypeMemory] == nil {
				t.Error("NewResourceManager() global memory quota not initialized")
			}
		})
	}
}

func TestResourceManager_SetGlobalQuota(t *testing.T) {
	rm := NewResourceManager(nil)

	tests := []struct {
		name        string
		cpuLimit    string
		memoryLimit string
		diskLimit   string
		wantErr     bool
	}{
		{
			name:        "valid limits",
			cpuLimit:    "4.0",
			memoryLimit: "8Gi",
			diskLimit:   "100Gi",
			wantErr:     false,
		},
		{
			name:        "empty limits",
			cpuLimit:    "",
			memoryLimit: "",
			diskLimit:   "",
			wantErr:     false,
		},
		{
			name:        "invalid CPU",
			cpuLimit:    "invalid",
			memoryLimit: "",
			diskLimit:   "",
			wantErr:     true,
		},
		{
			name:        "invalid memory",
			cpuLimit:    "",
			memoryLimit: "invalid",
			diskLimit:   "",
			wantErr:     true,
		},
		{
			name:        "invalid disk",
			cpuLimit:    "",
			memoryLimit: "",
			diskLimit:   "invalid",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rm.SetGlobalQuota(tt.cpuLimit, tt.memoryLimit, tt.diskLimit)

			if (err != nil) != tt.wantErr {
				t.Errorf("SetGlobalQuota() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.cpuLimit != "" {
				if rm.globalQuota.Global[ResourceTypeCPU].OriginalSpec != tt.cpuLimit {
					t.Errorf("SetGlobalQuota() CPU limit not set correctly")
				}
			}
		})
	}
}

func TestResourceManager_SetRepositoryQuota(t *testing.T) {
	rm := NewResourceManager(nil)

	tests := []struct {
		name      string
		repoName  string
		resources config.Resources
		wantErr   bool
	}{
		{
			name:     "valid repository quota",
			repoName: "test-repo",
			resources: config.Resources{
				CPULimit:  "2.0",
				MemLimit:  "4Gi",
				DiskLimit: "50Gi",
			},
			wantErr: false,
		},
		{
			name:      "empty resources",
			repoName:  "test-repo-2",
			resources: config.Resources{},
			wantErr:   false,
		},
		{
			name:     "invalid CPU",
			repoName: "test-repo-3",
			resources: config.Resources{
				CPULimit: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rm.SetRepositoryQuota(tt.repoName, tt.resources)

			if (err != nil) != tt.wantErr {
				t.Errorf("SetRepositoryQuota() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				quota, exists := rm.repositoryQuotas[tt.repoName]
				if !exists {
					t.Errorf("SetRepositoryQuota() repository quota not stored")
				}

				if tt.resources.CPULimit != "" {
					if quota.Repository[ResourceTypeCPU] == nil {
						t.Errorf("SetRepositoryQuota() CPU limit not set")
					}
				}
			}
		})
	}
}

func TestResourceManager_ValidateResourceRequest(t *testing.T) {
	rm := NewResourceManager(nil)

	// Set up test repository quota
	err := rm.SetRepositoryQuota("test-repo", config.Resources{
		CPULimit: "2.0",
		MemLimit: "2Gi",
	})
	if err != nil {
		t.Fatalf("Failed to set repository quota: %v", err)
	}

	tests := []struct {
		name          string
		repoName      string
		stepID        string
		cpuRequest    string
		memoryRequest string
		wantErr       bool
	}{
		{
			name:          "valid request within limits",
			repoName:      "test-repo",
			stepID:        "step-1",
			cpuRequest:    "1.0",
			memoryRequest: "500Mi",
			wantErr:       false,
		},
		{
			name:          "CPU exceeds repository limit",
			repoName:      "test-repo",
			stepID:        "step-2",
			cpuRequest:    "3.0",
			memoryRequest: "1Gi",
			wantErr:       true,
		},
		{
			name:          "memory exceeds repository limit",
			repoName:      "test-repo",
			stepID:        "step-3",
			cpuRequest:    "1.0",
			memoryRequest: "3Gi",
			wantErr:       true,
		},
		{
			name:          "invalid CPU request format",
			repoName:      "test-repo",
			stepID:        "step-4",
			cpuRequest:    "invalid",
			memoryRequest: "1Gi",
			wantErr:       true,
		},
		{
			name:          "empty requests",
			repoName:      "test-repo",
			stepID:        "step-5",
			cpuRequest:    "",
			memoryRequest: "",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rm.ValidateResourceRequest(tt.repoName, tt.stepID, tt.cpuRequest, tt.memoryRequest)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateResourceRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResourceManager_HierarchicalLimits(t *testing.T) {
	rm := NewResourceManager(nil)

	// Set global limits
	err := rm.SetGlobalQuota("8.0", "16Gi", "")
	if err != nil {
		t.Fatalf("Failed to set global quota: %v", err)
	}

	// Set repository limits (should be lower than global)
	err = rm.SetRepositoryQuota("test-repo", config.Resources{
		CPULimit: "4.0",
		MemLimit: "8Gi",
	})
	if err != nil {
		t.Fatalf("Failed to set repository quota: %v", err)
	}

	tests := []struct {
		name          string
		repoName      string
		stepID        string
		cpuRequest    string
		memoryRequest string
		wantErr       bool
		description   string
	}{
		{
			name:          "within all limits",
			repoName:      "test-repo",
			stepID:        "step-1",
			cpuRequest:    "0.5",
			memoryRequest: "512Mi",
			wantErr:       false,
			description:   "Request within step, repository, and global limits",
		},
		{
			name:          "exceeds step default but within repo",
			repoName:      "test-repo",
			stepID:        "step-2",
			cpuRequest:    "2.0",
			memoryRequest: "1Gi",
			wantErr:       false,
			description:   "Request exceeds default step limit but within repository limit",
		},
		{
			name:          "exceeds repository limit",
			repoName:      "test-repo",
			stepID:        "step-3",
			cpuRequest:    "5.0",
			memoryRequest: "4Gi",
			wantErr:       true,
			description:   "Request exceeds repository CPU limit",
		},
		{
			name:          "exceeds global limit",
			repoName:      "new-repo", // No specific quota, uses defaults
			stepID:        "step-4",
			cpuRequest:    "10.0",
			memoryRequest: "20Gi",
			wantErr:       true,
			description:   "Request exceeds global limits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rm.ValidateResourceRequest(tt.repoName, tt.stepID, tt.cpuRequest, tt.memoryRequest)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateResourceRequest() error = %v, wantErr %v\nDescription: %s",
					err, tt.wantErr, tt.description)
			}
		})
	}
}

func TestResourceManager_Monitoring(t *testing.T) {
	config := &ResourceManagerConfig{
		WarningThreshold:   0.8,
		MonitoringInterval: 100 * time.Millisecond, // Fast for testing
		MaxHistoryEntries:  5,
		Debug:              false,
	}
	rm := NewResourceManager(config)

	// Test monitoring start/stop
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if rm.monitorRunning {
		t.Error("Monitoring should not be running initially")
	}

	err := rm.StartMonitoring(ctx)
	if err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}

	if !rm.monitorRunning {
		t.Error("Monitoring should be running after start")
	}

	// Test starting again (should fail)
	err = rm.StartMonitoring(ctx)
	if err == nil {
		t.Error("Starting monitoring twice should return an error")
	}

	// Wait for some monitoring cycles
	time.Sleep(300 * time.Millisecond)

	// Check that usage history is being collected
	cpuHistory := rm.GetUsageHistory(ResourceTypeCPU)
	if len(cpuHistory) == 0 {
		t.Error("CPU usage history should have entries after monitoring")
	}

	memoryHistory := rm.GetUsageHistory(ResourceTypeMemory)
	if len(memoryHistory) == 0 {
		t.Error("Memory usage history should have entries after monitoring")
	}

	// Test current usage retrieval
	currentCPU := rm.GetCurrentUsage(ResourceTypeCPU)
	if currentCPU == nil {
		t.Error("Should be able to get current CPU usage")
	}

	currentMemory := rm.GetCurrentUsage(ResourceTypeMemory)
	if currentMemory == nil {
		t.Error("Should be able to get current memory usage")
	}

	// Stop monitoring
	rm.StopMonitoring()

	if rm.monitorRunning {
		t.Error("Monitoring should not be running after stop")
	}
}

func TestResourceManager_Callbacks(t *testing.T) {
	config := &ResourceManagerConfig{
		WarningThreshold:   0.5, // Low threshold for easy testing
		MonitoringInterval: 50 * time.Millisecond,
		MaxHistoryEntries:  10,
		Debug:              false,
	}
	rm := NewResourceManager(config)

	// Set up callback tracking
	var warningCalled, breachCalled bool

	rm.SetWarningCallback(func(resourceType ResourceType, usage *ResourceUsage) {
		warningCalled = true
	})

	rm.SetBreachCallback(func(resourceType ResourceType, usage *ResourceUsage, limit *ResourceLimit) {
		breachCalled = true
	})

	// Avoid unused variable warnings
	_ = warningCalled
	_ = breachCalled

	// Verify callbacks are set
	if rm.onWarning == nil {
		t.Error("Warning callback should be set")
	}
	if rm.onBreach == nil {
		t.Error("Breach callback should be set")
	}

	// Note: Testing actual callback invocation would require mocking system metrics
	// or manually triggering the callback conditions, which is complex for this unit test.
	// In a real implementation, you might want integration tests for callback functionality.
}

func TestResourceManager_GetMethods(t *testing.T) {
	rm := NewResourceManager(nil)

	// Test getting usage when no history exists
	currentUsage := rm.GetCurrentUsage(ResourceTypeCPU)
	if currentUsage != nil {
		t.Error("Should return nil when no usage history exists")
	}

	history := rm.GetUsageHistory(ResourceTypeCPU)
	if len(history) != 0 {
		t.Error("Should return empty history when no monitoring has occurred")
	}

	// Add some mock history for testing
	rm.mu.Lock()
	rm.usageHistory[ResourceTypeCPU] = []*ResourceUsage{
		{
			Type:        ResourceTypeCPU,
			Used:        1.0,
			Available:   4.0,
			Percentage:  25.0,
			LastUpdated: time.Now(),
		},
		{
			Type:        ResourceTypeCPU,
			Used:        2.0,
			Available:   4.0,
			Percentage:  50.0,
			LastUpdated: time.Now(),
		},
	}
	rm.mu.Unlock()

	// Test getting current usage
	currentUsage = rm.GetCurrentUsage(ResourceTypeCPU)
	if currentUsage == nil {
		t.Error("Should return current usage when history exists")
	}
	if currentUsage.Percentage != 50.0 {
		t.Errorf("Current usage percentage = %v, want 50.0", currentUsage.Percentage)
	}

	// Test getting history
	history = rm.GetUsageHistory(ResourceTypeCPU)
	if len(history) != 2 {
		t.Errorf("History length = %v, want 2", len(history))
	}
}
