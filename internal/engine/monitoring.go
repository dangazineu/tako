package engine

import (
	"fmt"
	"sync"
	"time"
)

// FanOutMetrics contains metrics for fan-out operations.
type FanOutMetrics struct {
	// Execution counts
	TotalFanOuts      int64 `json:"total_fanouts"`
	SuccessfulFanOuts int64 `json:"successful_fanouts"`
	FailedFanOuts     int64 `json:"failed_fanouts"`

	// Child execution metrics
	TotalChildren      int64 `json:"total_children"`
	SuccessfulChildren int64 `json:"successful_children"`
	FailedChildren     int64 `json:"failed_children"`
	TimedOutChildren   int64 `json:"timed_out_children"`

	// Latency metrics (in milliseconds)
	FanOutLatencyP50 float64 `json:"fanout_latency_p50"`
	FanOutLatencyP95 float64 `json:"fanout_latency_p95"`
	FanOutLatencyP99 float64 `json:"fanout_latency_p99"`

	ChildLatencyP50 float64 `json:"child_latency_p50"`
	ChildLatencyP95 float64 `json:"child_latency_p95"`
	ChildLatencyP99 float64 `json:"child_latency_p99"`

	// Error rates (percentage)
	FanOutErrorRate float64 `json:"fanout_error_rate"`
	ChildErrorRate  float64 `json:"child_error_rate"`

	// Concurrency metrics
	ActiveFanOuts         int64 `json:"active_fanouts"`
	ActiveChildren        int64 `json:"active_children"`
	MaxConcurrentFanOuts  int64 `json:"max_concurrent_fanouts"`
	MaxConcurrentChildren int64 `json:"max_concurrent_children"`

	// Resource utilization
	AverageChildrenPerFanOut float64   `json:"average_children_per_fanout"`
	LastUpdated              time.Time `json:"last_updated"`
}

// MetricsCollector collects and aggregates metrics for fan-out operations.
type MetricsCollector struct {
	metrics         FanOutMetrics
	fanOutLatencies []time.Duration
	childLatencies  []time.Duration
	mu              sync.RWMutex
	maxSamples      int // Maximum number of latency samples to keep
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		maxSamples:      1000, // Keep last 1000 samples for percentile calculation
		fanOutLatencies: make([]time.Duration, 0, 1000),
		childLatencies:  make([]time.Duration, 0, 1000),
	}
}

// RecordFanOutStarted records the start of a fan-out operation.
func (mc *MetricsCollector) RecordFanOutStarted() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.metrics.TotalFanOuts++
	mc.metrics.ActiveFanOuts++
	if mc.metrics.ActiveFanOuts > mc.metrics.MaxConcurrentFanOuts {
		mc.metrics.MaxConcurrentFanOuts = mc.metrics.ActiveFanOuts
	}
	mc.metrics.LastUpdated = time.Now()
}

// RecordFanOutCompleted records the completion of a fan-out operation.
func (mc *MetricsCollector) RecordFanOutCompleted(duration time.Duration, success bool, childrenCount int) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.metrics.ActiveFanOuts--
	if success {
		mc.metrics.SuccessfulFanOuts++
	} else {
		mc.metrics.FailedFanOuts++
	}

	// Record latency
	mc.addFanOutLatency(duration)

	// Update children metrics
	mc.metrics.TotalChildren += int64(childrenCount)

	// Update error rates
	mc.updateErrorRates()

	// Update average children per fan-out
	if mc.metrics.TotalFanOuts > 0 {
		mc.metrics.AverageChildrenPerFanOut = float64(mc.metrics.TotalChildren) / float64(mc.metrics.TotalFanOuts)
	}

	mc.metrics.LastUpdated = time.Now()
}

// RecordChildStarted records the start of a child workflow execution.
func (mc *MetricsCollector) RecordChildStarted() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.metrics.ActiveChildren++
	if mc.metrics.ActiveChildren > mc.metrics.MaxConcurrentChildren {
		mc.metrics.MaxConcurrentChildren = mc.metrics.ActiveChildren
	}
	mc.metrics.LastUpdated = time.Now()
}

// RecordChildCompleted records the completion of a child workflow execution.
func (mc *MetricsCollector) RecordChildCompleted(duration time.Duration, status ChildWorkflowStatus) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.metrics.ActiveChildren--

	switch status {
	case ChildStatusCompleted:
		mc.metrics.SuccessfulChildren++
	case ChildStatusFailed:
		mc.metrics.FailedChildren++
	case ChildStatusTimedOut:
		mc.metrics.TimedOutChildren++
	}

	// Record child latency
	mc.addChildLatency(duration)

	// Update error rates
	mc.updateErrorRates()

	mc.metrics.LastUpdated = time.Now()
}

// addFanOutLatency adds a fan-out latency sample and updates percentiles.
func (mc *MetricsCollector) addFanOutLatency(duration time.Duration) {
	mc.fanOutLatencies = append(mc.fanOutLatencies, duration)

	// Keep only the most recent samples
	if len(mc.fanOutLatencies) > mc.maxSamples {
		mc.fanOutLatencies = mc.fanOutLatencies[len(mc.fanOutLatencies)-mc.maxSamples:]
	}

	// Update percentiles
	mc.updateFanOutPercentiles()
}

// addChildLatency adds a child latency sample and updates percentiles.
func (mc *MetricsCollector) addChildLatency(duration time.Duration) {
	mc.childLatencies = append(mc.childLatencies, duration)

	// Keep only the most recent samples
	if len(mc.childLatencies) > mc.maxSamples {
		mc.childLatencies = mc.childLatencies[len(mc.childLatencies)-mc.maxSamples:]
	}

	// Update percentiles
	mc.updateChildPercentiles()
}

// updateFanOutPercentiles calculates and updates fan-out latency percentiles.
func (mc *MetricsCollector) updateFanOutPercentiles() {
	if len(mc.fanOutLatencies) == 0 {
		return
	}

	// Create a copy and sort
	sorted := make([]time.Duration, len(mc.fanOutLatencies))
	copy(sorted, mc.fanOutLatencies)

	// Simple insertion sort for small arrays
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}

	// Calculate percentiles
	mc.metrics.FanOutLatencyP50 = float64(sorted[len(sorted)*50/100]) / float64(time.Millisecond)
	mc.metrics.FanOutLatencyP95 = float64(sorted[len(sorted)*95/100]) / float64(time.Millisecond)
	mc.metrics.FanOutLatencyP99 = float64(sorted[len(sorted)*99/100]) / float64(time.Millisecond)
}

// updateChildPercentiles calculates and updates child latency percentiles.
func (mc *MetricsCollector) updateChildPercentiles() {
	if len(mc.childLatencies) == 0 {
		return
	}

	// Create a copy and sort
	sorted := make([]time.Duration, len(mc.childLatencies))
	copy(sorted, mc.childLatencies)

	// Simple insertion sort for small arrays
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}

	// Calculate percentiles
	mc.metrics.ChildLatencyP50 = float64(sorted[len(sorted)*50/100]) / float64(time.Millisecond)
	mc.metrics.ChildLatencyP95 = float64(sorted[len(sorted)*95/100]) / float64(time.Millisecond)
	mc.metrics.ChildLatencyP99 = float64(sorted[len(sorted)*99/100]) / float64(time.Millisecond)
}

// updateErrorRates calculates and updates error rates.
func (mc *MetricsCollector) updateErrorRates() {
	// Fan-out error rate
	if mc.metrics.TotalFanOuts > 0 {
		mc.metrics.FanOutErrorRate = float64(mc.metrics.FailedFanOuts) / float64(mc.metrics.TotalFanOuts) * 100.0
	}

	// Child error rate
	totalCompletedChildren := mc.metrics.SuccessfulChildren + mc.metrics.FailedChildren + mc.metrics.TimedOutChildren
	if totalCompletedChildren > 0 {
		failedChildren := mc.metrics.FailedChildren + mc.metrics.TimedOutChildren
		mc.metrics.ChildErrorRate = float64(failedChildren) / float64(totalCompletedChildren) * 100.0
	}
}

// GetMetrics returns a snapshot of current metrics.
func (mc *MetricsCollector) GetMetrics() FanOutMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.metrics
}

// Reset resets all metrics to zero.
func (mc *MetricsCollector) Reset() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.metrics = FanOutMetrics{
		LastUpdated: time.Now(),
	}
	mc.fanOutLatencies = mc.fanOutLatencies[:0]
	mc.childLatencies = mc.childLatencies[:0]
}

// HealthStatus represents the health status of the fan-out system.
type HealthStatus struct {
	Status            string            `json:"status"`            // "healthy", "degraded", "unhealthy"
	ErrorRate         float64           `json:"error_rate"`        // Overall error rate percentage
	AverageLatency    float64           `json:"average_latency"`   // Average latency in milliseconds
	ActiveOperations  int64             `json:"active_operations"` // Number of active operations
	CircuitBreakers   map[string]string `json:"circuit_breakers"`  // Status of circuit breakers by endpoint
	LastHealthCheck   time.Time         `json:"last_health_check"`
	HealthCheckErrors []string          `json:"health_check_errors,omitempty"`
}

// HealthChecker performs health checks on the fan-out system.
type HealthChecker struct {
	metricsCollector      *MetricsCollector
	circuitBreakerManager *CircuitBreakerManager

	// Health thresholds
	errorRateThreshold float64 // Maximum acceptable error rate percentage
	latencyThreshold   float64 // Maximum acceptable latency in milliseconds
	activeOpsThreshold int64   // Maximum acceptable active operations
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(metricsCollector *MetricsCollector, circuitBreakerManager *CircuitBreakerManager) *HealthChecker {
	return &HealthChecker{
		metricsCollector:      metricsCollector,
		circuitBreakerManager: circuitBreakerManager,
		errorRateThreshold:    10.0,   // 10% error rate threshold
		latencyThreshold:      5000.0, // 5 second latency threshold
		activeOpsThreshold:    100,    // 100 active operations threshold
	}
}

// CheckHealth performs a comprehensive health check.
func (hc *HealthChecker) CheckHealth() HealthStatus {
	status := HealthStatus{
		Status:            "healthy",
		LastHealthCheck:   time.Now(),
		CircuitBreakers:   make(map[string]string),
		HealthCheckErrors: make([]string, 0),
	}

	// Get current metrics
	metrics := hc.metricsCollector.GetMetrics()

	// Count issues to determine severity
	issueCount := 0

	// Check error rates
	if metrics.FanOutErrorRate > hc.errorRateThreshold {
		issueCount++
		status.HealthCheckErrors = append(status.HealthCheckErrors,
			fmt.Sprintf("High fan-out error rate: %.2f%%", metrics.FanOutErrorRate))
	}
	if metrics.ChildErrorRate > hc.errorRateThreshold {
		issueCount++
		status.HealthCheckErrors = append(status.HealthCheckErrors,
			fmt.Sprintf("High child error rate: %.2f%%", metrics.ChildErrorRate))
	}

	// Check latencies
	if metrics.FanOutLatencyP95 > hc.latencyThreshold || metrics.ChildLatencyP95 > hc.latencyThreshold {
		issueCount++
		if metrics.FanOutLatencyP95 > hc.latencyThreshold {
			status.HealthCheckErrors = append(status.HealthCheckErrors,
				fmt.Sprintf("High fan-out latency P95: %.2fms", metrics.FanOutLatencyP95))
		}
		if metrics.ChildLatencyP95 > hc.latencyThreshold {
			status.HealthCheckErrors = append(status.HealthCheckErrors,
				fmt.Sprintf("High child latency P95: %.2fms", metrics.ChildLatencyP95))
		}
	}

	// Check active operations
	totalActiveOps := metrics.ActiveFanOuts + metrics.ActiveChildren
	if totalActiveOps > hc.activeOpsThreshold {
		issueCount++
		status.HealthCheckErrors = append(status.HealthCheckErrors,
			fmt.Sprintf("High active operations: %d", totalActiveOps))
	}

	// Check circuit breaker states
	cbStats := hc.circuitBreakerManager.GetAllStats()
	for endpoint, stats := range cbStats {
		status.CircuitBreakers[endpoint] = stats.State.String()
		if stats.State == CircuitBreakerOpen {
			issueCount++
			if status.Status == "healthy" {
				status.Status = "degraded"
			}
			status.HealthCheckErrors = append(status.HealthCheckErrors,
				fmt.Sprintf("Circuit breaker open for endpoint: %s", endpoint))
		}
	}

	// Escalate to unhealthy based on issue count
	if issueCount >= 3 {
		status.Status = "unhealthy"
	} else if issueCount >= 1 && status.Status == "healthy" {
		status.Status = "degraded"
	}

	// Set summary metrics
	status.ErrorRate = (metrics.FanOutErrorRate + metrics.ChildErrorRate) / 2.0
	status.AverageLatency = (metrics.FanOutLatencyP50 + metrics.ChildLatencyP50) / 2.0
	status.ActiveOperations = totalActiveOps

	return status
}

// SetThresholds allows customization of health check thresholds.
func (hc *HealthChecker) SetThresholds(errorRate, latency float64, activeOps int64) {
	hc.errorRateThreshold = errorRate
	hc.latencyThreshold = latency
	hc.activeOpsThreshold = activeOps
}

// Logger interface for structured logging.
type Logger interface {
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
}

// StructuredLogger provides structured logging for fan-out operations.
type StructuredLogger struct {
	enableDebug bool
}

// NewStructuredLogger creates a new structured logger.
func NewStructuredLogger(enableDebug bool) *StructuredLogger {
	return &StructuredLogger{
		enableDebug: enableDebug,
	}
}

// Info logs an info message with structured fields.
func (sl *StructuredLogger) Info(msg string, fields ...interface{}) {
	fmt.Printf("[INFO] %s %v\n", msg, fields)
}

// Warn logs a warning message with structured fields.
func (sl *StructuredLogger) Warn(msg string, fields ...interface{}) {
	fmt.Printf("[WARN] %s %v\n", msg, fields)
}

// Error logs an error message with structured fields.
func (sl *StructuredLogger) Error(msg string, fields ...interface{}) {
	fmt.Printf("[ERROR] %s %v\n", msg, fields)
}

// Debug logs a debug message with structured fields.
func (sl *StructuredLogger) Debug(msg string, fields ...interface{}) {
	if sl.enableDebug {
		fmt.Printf("[DEBUG] %s %v\n", msg, fields)
	}
}
