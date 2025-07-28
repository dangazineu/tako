package engine

import (
	"testing"
	"time"
)

func TestNewMetricsCollector(t *testing.T) {
	mc := NewMetricsCollector()
	if mc == nil {
		t.Fatal("Expected metrics collector to be created")
	}

	metrics := mc.GetMetrics()
	if metrics.TotalFanOuts != 0 {
		t.Errorf("Expected initial total fan-outs to be 0, got %d", metrics.TotalFanOuts)
	}
}

func TestMetricsCollectorFanOutOperations(t *testing.T) {
	mc := NewMetricsCollector()

	// Record fan-out operations
	mc.RecordFanOutStarted()
	mc.RecordFanOutStarted()

	metrics := mc.GetMetrics()
	if metrics.TotalFanOuts != 2 {
		t.Errorf("Expected 2 total fan-outs, got %d", metrics.TotalFanOuts)
	}
	if metrics.ActiveFanOuts != 2 {
		t.Errorf("Expected 2 active fan-outs, got %d", metrics.ActiveFanOuts)
	}

	// Complete one successfully
	mc.RecordFanOutCompleted(100*time.Millisecond, true, 3)
	metrics = mc.GetMetrics()
	if metrics.SuccessfulFanOuts != 1 {
		t.Errorf("Expected 1 successful fan-out, got %d", metrics.SuccessfulFanOuts)
	}
	if metrics.ActiveFanOuts != 1 {
		t.Errorf("Expected 1 active fan-out, got %d", metrics.ActiveFanOuts)
	}
	if metrics.TotalChildren != 3 {
		t.Errorf("Expected 3 total children, got %d", metrics.TotalChildren)
	}

	// Complete one with failure
	mc.RecordFanOutCompleted(200*time.Millisecond, false, 2)
	metrics = mc.GetMetrics()
	if metrics.FailedFanOuts != 1 {
		t.Errorf("Expected 1 failed fan-out, got %d", metrics.FailedFanOuts)
	}
	if metrics.ActiveFanOuts != 0 {
		t.Errorf("Expected 0 active fan-outs, got %d", metrics.ActiveFanOuts)
	}
	if metrics.TotalChildren != 5 {
		t.Errorf("Expected 5 total children, got %d", metrics.TotalChildren)
	}
}

func TestMetricsCollectorChildOperations(t *testing.T) {
	mc := NewMetricsCollector()

	// Record child operations
	mc.RecordChildStarted()
	mc.RecordChildStarted()
	mc.RecordChildStarted()

	metrics := mc.GetMetrics()
	if metrics.ActiveChildren != 3 {
		t.Errorf("Expected 3 active children, got %d", metrics.ActiveChildren)
	}

	// Complete children with different statuses
	mc.RecordChildCompleted(50*time.Millisecond, ChildStatusCompleted)
	mc.RecordChildCompleted(100*time.Millisecond, ChildStatusFailed)
	mc.RecordChildCompleted(75*time.Millisecond, ChildStatusTimedOut)

	metrics = mc.GetMetrics()
	if metrics.ActiveChildren != 0 {
		t.Errorf("Expected 0 active children, got %d", metrics.ActiveChildren)
	}
	if metrics.SuccessfulChildren != 1 {
		t.Errorf("Expected 1 successful child, got %d", metrics.SuccessfulChildren)
	}
	if metrics.FailedChildren != 1 {
		t.Errorf("Expected 1 failed child, got %d", metrics.FailedChildren)
	}
	if metrics.TimedOutChildren != 1 {
		t.Errorf("Expected 1 timed out child, got %d", metrics.TimedOutChildren)
	}
}

func TestMetricsCollectorPercentiles(t *testing.T) {
	mc := NewMetricsCollector()

	// Add fan-out latencies: 100ms, 200ms, 300ms, 400ms, 500ms
	for i := 1; i <= 5; i++ {
		duration := time.Duration(i*100) * time.Millisecond
		mc.addFanOutLatency(duration)
	}

	metrics := mc.GetMetrics()

	// P50 should be around 300ms (middle value)
	expectedP50 := 300.0
	if metrics.FanOutLatencyP50 != expectedP50 {
		t.Errorf("Expected P50 latency %.1fms, got %.1fms", expectedP50, metrics.FanOutLatencyP50)
	}

	// P95 should be around 500ms (95th percentile of 5 values = index 4)
	expectedP95 := 500.0
	if metrics.FanOutLatencyP95 != expectedP95 {
		t.Errorf("Expected P95 latency %.1fms, got %.1fms", expectedP95, metrics.FanOutLatencyP95)
	}
}

func TestMetricsCollectorErrorRates(t *testing.T) {
	mc := NewMetricsCollector()

	// Record fan-outs: 7 successful, 3 failed
	for i := 0; i < 7; i++ {
		mc.RecordFanOutStarted()
		mc.RecordFanOutCompleted(100*time.Millisecond, true, 1)
	}
	for i := 0; i < 3; i++ {
		mc.RecordFanOutStarted()
		mc.RecordFanOutCompleted(100*time.Millisecond, false, 1)
	}

	metrics := mc.GetMetrics()
	expectedErrorRate := 30.0 // 3/10 * 100
	if metrics.FanOutErrorRate != expectedErrorRate {
		t.Errorf("Expected fan-out error rate %.1f%%, got %.1f%%", expectedErrorRate, metrics.FanOutErrorRate)
	}

	// Record children: 8 successful, 2 failed
	for i := 0; i < 8; i++ {
		mc.RecordChildStarted()
		mc.RecordChildCompleted(50*time.Millisecond, ChildStatusCompleted)
	}
	for i := 0; i < 2; i++ {
		mc.RecordChildStarted()
		mc.RecordChildCompleted(50*time.Millisecond, ChildStatusFailed)
	}

	metrics = mc.GetMetrics()
	expectedChildErrorRate := 20.0 // 2/10 * 100
	if metrics.ChildErrorRate != expectedChildErrorRate {
		t.Errorf("Expected child error rate %.1f%%, got %.1f%%", expectedChildErrorRate, metrics.ChildErrorRate)
	}
}

func TestMetricsCollectorReset(t *testing.T) {
	mc := NewMetricsCollector()

	// Add some data
	mc.RecordFanOutStarted()
	mc.RecordFanOutCompleted(100*time.Millisecond, true, 2)
	mc.RecordChildStarted()
	mc.RecordChildCompleted(50*time.Millisecond, ChildStatusCompleted)

	// Reset
	mc.Reset()

	metrics := mc.GetMetrics()
	if metrics.TotalFanOuts != 0 {
		t.Errorf("Expected total fan-outs to be 0 after reset, got %d", metrics.TotalFanOuts)
	}
	if metrics.TotalChildren != 0 {
		t.Errorf("Expected total children to be 0 after reset, got %d", metrics.TotalChildren)
	}
	if metrics.FanOutLatencyP50 != 0 {
		t.Errorf("Expected P50 latency to be 0 after reset, got %.1f", metrics.FanOutLatencyP50)
	}
}

func TestHealthChecker(t *testing.T) {
	mc := NewMetricsCollector()
	cbm := NewCircuitBreakerManager(DefaultCircuitBreakerConfig())
	hc := NewHealthChecker(mc, cbm)

	// Initial health should be healthy
	health := hc.CheckHealth()
	if health.Status != "healthy" {
		t.Errorf("Expected initial status to be healthy, got %s", health.Status)
	}
	if len(health.HealthCheckErrors) != 0 {
		t.Errorf("Expected no health check errors, got %d", len(health.HealthCheckErrors))
	}
}

func TestHealthCheckerDegraded(t *testing.T) {
	mc := NewMetricsCollector()
	cbm := NewCircuitBreakerManager(DefaultCircuitBreakerConfig())
	hc := NewHealthChecker(mc, cbm)

	// Set low thresholds for testing
	hc.SetThresholds(5.0, 100.0, 5) // 5% error rate, 100ms latency, 5 active ops

	// Create high error rate scenario
	for i := 0; i < 8; i++ {
		mc.RecordFanOutStarted()
		mc.RecordFanOutCompleted(50*time.Millisecond, true, 1)
	}
	for i := 0; i < 2; i++ {
		mc.RecordFanOutStarted()
		mc.RecordFanOutCompleted(50*time.Millisecond, false, 1) // 20% error rate
	}

	health := hc.CheckHealth()
	if health.Status != "degraded" {
		t.Errorf("Expected status to be degraded due to high error rate, got %s", health.Status)
	}
	if len(health.HealthCheckErrors) == 0 {
		t.Error("Expected health check errors for high error rate")
	}
}

func TestHealthCheckerUnhealthy(t *testing.T) {
	mc := NewMetricsCollector()
	cbm := NewCircuitBreakerManager(DefaultCircuitBreakerConfig())
	hc := NewHealthChecker(mc, cbm)

	// Set low thresholds
	hc.SetThresholds(5.0, 50.0, 2)

	// Create high error rate
	for i := 0; i < 5; i++ {
		mc.RecordFanOutStarted()
		mc.RecordFanOutCompleted(50*time.Millisecond, false, 1) // 100% error rate
	}

	// Create high latency
	mc.addFanOutLatency(1000 * time.Millisecond) // Very high latency

	// Create high active operations
	for i := 0; i < 5; i++ {
		mc.RecordFanOutStarted() // Don't complete them to keep them active
	}

	health := hc.CheckHealth()
	if health.Status != "unhealthy" {
		t.Errorf("Expected status to be unhealthy, got %s", health.Status)
	}
	if len(health.HealthCheckErrors) < 2 {
		t.Errorf("Expected multiple health check errors, got %d", len(health.HealthCheckErrors))
	}
}

func TestHealthCheckerCircuitBreaker(t *testing.T) {
	mc := NewMetricsCollector()
	cbm := NewCircuitBreakerManager(CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxRequests:      2,
	})
	hc := NewHealthChecker(mc, cbm)

	// Open a circuit breaker
	cb := cbm.GetCircuitBreaker("test-endpoint")
	cb.Call(func() error { return NewHTTPError(500, "Internal Server Error") })

	health := hc.CheckHealth()
	if health.Status == "healthy" {
		t.Error("Expected status to be degraded due to open circuit breaker")
	}
	if health.CircuitBreakers["test-endpoint"] != "open" {
		t.Errorf("Expected circuit breaker to be open, got %s", health.CircuitBreakers["test-endpoint"])
	}
	if len(health.HealthCheckErrors) == 0 {
		t.Error("Expected health check errors for open circuit breaker")
	}
}

func TestStructuredLogger(t *testing.T) {
	// Test logger creation
	logger := NewStructuredLogger(true)
	if logger == nil {
		t.Fatal("Expected logger to be created")
	}

	// Test that logging methods don't panic
	logger.Info("test info", "key", "value")
	logger.Warn("test warn", "key", "value")
	logger.Error("test error", "key", "value")
	logger.Debug("test debug", "key", "value")

	// Test logger with debug disabled
	loggerNoDebug := NewStructuredLogger(false)
	loggerNoDebug.Debug("this should not appear", "key", "value")
}

func TestFanOutExecutorMonitoring(t *testing.T) {
	tempDir := t.TempDir()
	executor, err := NewFanOutExecutor(tempDir, false)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Test metrics access
	metrics := executor.GetMetrics()
	if metrics.TotalFanOuts != 0 {
		t.Errorf("Expected 0 initial fan-outs, got %d", metrics.TotalFanOuts)
	}

	// Test health status
	health := executor.GetHealthStatus()
	if health.Status != "healthy" {
		t.Errorf("Expected initial health to be healthy, got %s", health.Status)
	}

	// Test circuit breaker stats
	cbStats := executor.GetCircuitBreakerStats()
	if len(cbStats) != 0 {
		t.Errorf("Expected no initial circuit breaker stats, got %d", len(cbStats))
	}

	// Test configuration methods
	executor.SetHealthThresholds(15.0, 2000.0, 50)
	executor.ConfigureRetry(RetryConfig{MaxRetries: 5})
	executor.ConfigureCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 10})

	// Test reset methods
	executor.ResetMetrics()
	executor.ResetCircuitBreakers()

	// Verify metrics are reset
	metrics = executor.GetMetrics()
	if metrics.TotalFanOuts != 0 {
		t.Errorf("Expected metrics to be reset, got %d total fan-outs", metrics.TotalFanOuts)
	}
}

func TestMetricsCollectorConcurrency(t *testing.T) {
	mc := NewMetricsCollector()

	// Test concurrent access to metrics collector
	done := make(chan bool, 2)

	// Goroutine 1: Record fan-out operations
	go func() {
		for i := 0; i < 100; i++ {
			mc.RecordFanOutStarted()
			time.Sleep(1 * time.Millisecond)
			mc.RecordFanOutCompleted(time.Millisecond*time.Duration(i), i%2 == 0, 1)
		}
		done <- true
	}()

	// Goroutine 2: Record child operations
	go func() {
		for i := 0; i < 100; i++ {
			mc.RecordChildStarted()
			time.Sleep(1 * time.Millisecond)
			status := ChildStatusCompleted
			if i%3 == 0 {
				status = ChildStatusFailed
			}
			mc.RecordChildCompleted(time.Millisecond*time.Duration(i), status)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify no data races and final state is consistent
	metrics := mc.GetMetrics()
	if metrics.TotalFanOuts != 100 {
		t.Errorf("Expected 100 total fan-outs, got %d", metrics.TotalFanOuts)
	}
	if metrics.SuccessfulChildren+metrics.FailedChildren != 100 {
		t.Errorf("Expected 100 total completed children, got %d",
			metrics.SuccessfulChildren+metrics.FailedChildren)
	}
}
