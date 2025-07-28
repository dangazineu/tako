package engine

import (
	"errors"
	"testing"
	"time"
)

func TestNewCircuitBreaker(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)

	if cb == nil {
		t.Fatal("Expected circuit breaker to be created")
	}
	if cb.GetState() != CircuitBreakerClosed {
		t.Errorf("Expected initial state to be closed, got %v", cb.GetState())
	}
}

func TestCircuitBreakerStateTransitions(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxRequests:      2,
	}
	cb := NewCircuitBreaker(config)

	// Test closed -> open transition
	failingFn := func() error { return errors.New("test error") }

	// First failure - should remain closed
	err := cb.Call(failingFn)
	if err == nil {
		t.Error("Expected error from failing function")
	}
	if cb.GetState() != CircuitBreakerClosed {
		t.Errorf("Expected state to remain closed after 1 failure, got %v", cb.GetState())
	}

	// Second failure - should open
	err = cb.Call(failingFn)
	if err == nil {
		t.Error("Expected error from failing function")
	}
	if cb.GetState() != CircuitBreakerOpen {
		t.Errorf("Expected state to be open after %d failures, got %v", config.FailureThreshold, cb.GetState())
	}

	// Third call should fail immediately due to open circuit
	err = cb.Call(func() error { return nil })
	if err == nil {
		t.Error("Expected circuit breaker to block call when open")
	}
	if err.Error() != "circuit breaker is open" {
		t.Errorf("Expected circuit breaker error, got: %v", err)
	}
}

func TestCircuitBreakerHalfOpenTransition(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
		MaxRequests:      2,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Call(func() error { return errors.New("test error") })
	if cb.GetState() != CircuitBreakerOpen {
		t.Fatal("Expected circuit to be open")
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Next call should transition to half-open
	successFn := func() error { return nil }
	err := cb.Call(successFn)
	if err != nil {
		t.Errorf("Expected successful call in half-open state, got: %v", err)
	}
	if cb.GetState() != CircuitBreakerHalfOpen {
		t.Errorf("Expected state to be half-open, got %v", cb.GetState())
	}
}

func TestCircuitBreakerHalfOpenToOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
		MaxRequests:      2,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Call(func() error { return errors.New("test error") })

	// Wait for timeout and transition to half-open
	time.Sleep(60 * time.Millisecond)
	cb.Call(func() error { return nil }) // First success in half-open

	// Failure in half-open should immediately open the circuit
	cb.Call(func() error { return errors.New("test error") })
	if cb.GetState() != CircuitBreakerOpen {
		t.Errorf("Expected circuit to reopen after failure in half-open state, got %v", cb.GetState())
	}
}

func TestCircuitBreakerHalfOpenToClosed(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
		MaxRequests:      3,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Call(func() error { return errors.New("test error") })

	// Wait for timeout and transition to half-open
	time.Sleep(60 * time.Millisecond)

	// Two successes should close the circuit
	cb.Call(func() error { return nil })
	cb.Call(func() error { return nil })

	if cb.GetState() != CircuitBreakerClosed {
		t.Errorf("Expected circuit to be closed after %d successes, got %v", config.SuccessThreshold, cb.GetState())
	}
}

func TestCircuitBreakerMaxRequestsInHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 3,
		Timeout:          50 * time.Millisecond,
		MaxRequests:      2,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Call(func() error { return errors.New("test error") })

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// First two calls should succeed (within MaxRequests)
	err1 := cb.Call(func() error { return nil })
	err2 := cb.Call(func() error { return nil })

	if err1 != nil || err2 != nil {
		t.Error("Expected first two calls to succeed in half-open state")
	}

	// Third call should be blocked
	err3 := cb.Call(func() error { return nil })
	if err3 == nil {
		t.Error("Expected third call to be blocked due to MaxRequests limit")
	}
}

func TestCircuitBreakerStats(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)

	stats := cb.GetStats()
	if stats.State != CircuitBreakerClosed {
		t.Errorf("Expected initial state to be closed, got %v", stats.State)
	}
	if stats.Failures != 0 {
		t.Errorf("Expected initial failures to be 0, got %d", stats.Failures)
	}

	// Record some failures
	cb.Call(func() error { return errors.New("test error") })
	cb.Call(func() error { return errors.New("test error") })

	stats = cb.GetStats()
	if stats.Failures != 2 {
		t.Errorf("Expected 2 failures, got %d", stats.Failures)
	}
	if stats.LastFailureTime.IsZero() {
		t.Error("Expected LastFailureTime to be set")
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
		MaxRequests:      2,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Call(func() error { return errors.New("test error") })
	if cb.GetState() != CircuitBreakerOpen {
		t.Fatal("Expected circuit to be open")
	}

	// Reset should close the circuit
	cb.Reset()
	if cb.GetState() != CircuitBreakerClosed {
		t.Errorf("Expected circuit to be closed after reset, got %v", cb.GetState())
	}

	stats := cb.GetStats()
	if stats.Failures != 0 {
		t.Errorf("Expected failures to be reset to 0, got %d", stats.Failures)
	}
}

func TestCircuitBreakerManager(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cbm := NewCircuitBreakerManager(config)

	// Get circuit breaker for endpoint
	endpoint1 := "service1.example.com"
	cb1 := cbm.GetCircuitBreaker(endpoint1)
	if cb1 == nil {
		t.Error("Expected circuit breaker to be created")
	}

	// Getting the same endpoint should return the same instance
	cb1Again := cbm.GetCircuitBreaker(endpoint1)
	if cb1 != cb1Again {
		t.Error("Expected same circuit breaker instance for same endpoint")
	}

	// Different endpoint should get different circuit breaker
	endpoint2 := "service2.example.com"
	cb2 := cbm.GetCircuitBreaker(endpoint2)
	if cb1 == cb2 {
		t.Error("Expected different circuit breaker instances for different endpoints")
	}
}

func TestCircuitBreakerManagerStats(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cbm := NewCircuitBreakerManager(config)

	// Create circuit breakers for different endpoints
	cb1 := cbm.GetCircuitBreaker("service1")
	cb2 := cbm.GetCircuitBreaker("service2")

	// Generate some failures
	cb1.Call(func() error { return errors.New("error") })
	cb2.Call(func() error { return nil })

	// Get all stats
	allStats := cbm.GetAllStats()
	if len(allStats) != 2 {
		t.Errorf("Expected stats for 2 endpoints, got %d", len(allStats))
	}

	if stats, exists := allStats["service1"]; !exists {
		t.Error("Expected stats for service1")
	} else if stats.Failures != 1 {
		t.Errorf("Expected 1 failure for service1, got %d", stats.Failures)
	}

	if stats, exists := allStats["service2"]; !exists {
		t.Error("Expected stats for service2")
	} else if stats.Successes != 1 {
		t.Errorf("Expected 1 success for service2, got %d", stats.Successes)
	}
}

func TestCircuitBreakerManagerReset(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
		MaxRequests:      2,
	}
	cbm := NewCircuitBreakerManager(config)

	// Open a circuit
	cb := cbm.GetCircuitBreaker("service1")
	cb.Call(func() error { return errors.New("error") })
	if cb.GetState() != CircuitBreakerOpen {
		t.Fatal("Expected circuit to be open")
	}

	// Reset specific endpoint
	cbm.ResetEndpoint("service1")
	if cb.GetState() != CircuitBreakerClosed {
		t.Error("Expected circuit to be closed after reset")
	}

	// Open circuit again
	cb.Call(func() error { return errors.New("error") })

	// Reset all
	cbm.ResetAll()
	if cb.GetState() != CircuitBreakerClosed {
		t.Error("Expected circuit to be closed after reset all")
	}
}

func TestCircuitBreakerManagerCleanup(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cbm := NewCircuitBreakerManager(config)

	// Create circuit breakers
	cb1 := cbm.GetCircuitBreaker("old-service")
	cb2 := cbm.GetCircuitBreaker("recent-service")

	// Simulate old failure
	cb1.Call(func() error { return errors.New("old error") })
	time.Sleep(10 * time.Millisecond)

	// Recent activity
	cb2.Call(func() error { return nil })

	// Check initial count
	allStats := cbm.GetAllStats()
	if len(allStats) != 2 {
		t.Errorf("Expected 2 circuit breakers, got %d", len(allStats))
	}

	// Cleanup stale breakers (very short duration for testing)
	cbm.CleanupStaleBreakers(5 * time.Millisecond)

	// old-service should be cleaned up (it's closed and has old failure)
	// recent-service should remain (it has recent activity)
	allStats = cbm.GetAllStats()
	if len(allStats) != 1 {
		t.Errorf("Expected 1 circuit breaker after cleanup, got %d", len(allStats))
	}

	if _, exists := allStats["recent-service"]; !exists {
		t.Error("Expected recent-service to remain after cleanup")
	}
}

func TestCircuitBreakerStateString(t *testing.T) {
	tests := []struct {
		state    CircuitBreakerState
		expected string
	}{
		{CircuitBreakerClosed, "closed"},
		{CircuitBreakerOpen, "open"},
		{CircuitBreakerHalfOpen, "half-open"},
		{CircuitBreakerState(999), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("CircuitBreakerState(%v).String() = %v, want %v", tt.state, got, tt.expected)
		}
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	if config.FailureThreshold <= 0 {
		t.Error("Expected positive failure threshold")
	}
	if config.SuccessThreshold <= 0 {
		t.Error("Expected positive success threshold")
	}
	if config.Timeout <= 0 {
		t.Error("Expected positive timeout")
	}
	if config.MaxRequests <= 0 {
		t.Error("Expected positive max requests")
	}
}
