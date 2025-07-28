package engine

import (
	"fmt"
	"sync"
	"time"
)

// CircuitBreakerState represents the current state of a circuit breaker.
type CircuitBreakerState int

const (
	CircuitBreakerClosed   CircuitBreakerState = iota // Normal operation
	CircuitBreakerOpen                                // Failing, blocking requests
	CircuitBreakerHalfOpen                            // Testing if service has recovered
)

func (s CircuitBreakerState) String() string {
	switch s {
	case CircuitBreakerClosed:
		return "closed"
	case CircuitBreakerOpen:
		return "open"
	case CircuitBreakerHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig defines the configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	FailureThreshold int           `yaml:"failure_threshold"` // Number of failures before opening
	SuccessThreshold int           `yaml:"success_threshold"` // Number of successes to close from half-open
	Timeout          time.Duration `yaml:"timeout"`           // Time to wait before switching to half-open
	MaxRequests      int           `yaml:"max_requests"`      // Max requests allowed in half-open state
}

// DefaultCircuitBreakerConfig returns a sensible default configuration.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,                // Open after 5 consecutive failures
		SuccessThreshold: 3,                // Close after 3 consecutive successes in half-open
		Timeout:          30 * time.Second, // Wait 30s before trying again
		MaxRequests:      3,                // Allow 3 requests in half-open state
	}
}

// CircuitBreaker implements the circuit breaker pattern for preventing cascading failures.
type CircuitBreaker struct {
	config           CircuitBreakerConfig
	state            CircuitBreakerState
	failures         int
	successes        int
	lastFailureTime  time.Time
	halfOpenRequests int
	mu               sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  CircuitBreakerClosed,
	}
}

// Call executes a function with circuit breaker protection.
func (cb *CircuitBreaker) Call(fn func() error) error {
	if !cb.canExecute() {
		return fmt.Errorf("circuit breaker is open")
	}

	err := fn()
	cb.recordResult(err)
	return err
}

// canExecute determines if a request can be executed based on the current state.
func (cb *CircuitBreaker) canExecute() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitBreakerClosed:
		return true
	case CircuitBreakerOpen:
		// Check if timeout has passed to transition to half-open
		if time.Since(cb.lastFailureTime) >= cb.config.Timeout {
			cb.state = CircuitBreakerHalfOpen
			cb.halfOpenRequests = 0
			return true
		}
		return false
	case CircuitBreakerHalfOpen:
		// Allow limited requests in half-open state
		return cb.halfOpenRequests < cb.config.MaxRequests
	default:
		return false
	}
}

// recordResult updates the circuit breaker state based on the execution result.
func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
}

// onFailure handles a failed execution.
func (cb *CircuitBreaker) onFailure() {
	cb.failures++
	cb.successes = 0
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitBreakerClosed:
		if cb.failures >= cb.config.FailureThreshold {
			cb.state = CircuitBreakerOpen
		}
	case CircuitBreakerHalfOpen:
		// Any failure in half-open state immediately opens the circuit
		cb.state = CircuitBreakerOpen
		cb.halfOpenRequests = 0
	}
}

// onSuccess handles a successful execution.
func (cb *CircuitBreaker) onSuccess() {
	cb.successes++

	switch cb.state {
	case CircuitBreakerClosed:
		// Reset failure count on success
		cb.failures = 0
	case CircuitBreakerHalfOpen:
		cb.halfOpenRequests++
		if cb.successes >= cb.config.SuccessThreshold {
			// Close the circuit after enough successes
			cb.state = CircuitBreakerClosed
			cb.failures = 0
			cb.halfOpenRequests = 0
		}
	}
}

// GetState returns the current state of the circuit breaker.
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns statistics about the circuit breaker.
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:            cb.state,
		Failures:         cb.failures,
		Successes:        cb.successes,
		LastFailureTime:  cb.lastFailureTime,
		HalfOpenRequests: cb.halfOpenRequests,
		FailureThreshold: cb.config.FailureThreshold,
		SuccessThreshold: cb.config.SuccessThreshold,
		Timeout:          cb.config.Timeout,
		MaxRequests:      cb.config.MaxRequests,
	}
}

// Reset manually resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitBreakerClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0
	cb.lastFailureTime = time.Time{}
}

// CircuitBreakerStats contains statistics about a circuit breaker.
type CircuitBreakerStats struct {
	State            CircuitBreakerState `json:"state"`
	Failures         int                 `json:"failures"`
	Successes        int                 `json:"successes"`
	LastFailureTime  time.Time           `json:"last_failure_time"`
	HalfOpenRequests int                 `json:"half_open_requests"`
	FailureThreshold int                 `json:"failure_threshold"`
	SuccessThreshold int                 `json:"success_threshold"`
	Timeout          time.Duration       `json:"timeout"`
	MaxRequests      int                 `json:"max_requests"`
}

// CircuitBreakerManager manages circuit breakers for different endpoints.
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
	mu       sync.RWMutex
}

// NewCircuitBreakerManager creates a new circuit breaker manager.
func NewCircuitBreakerManager(config CircuitBreakerConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// GetCircuitBreaker returns the circuit breaker for a given endpoint.
func (cbm *CircuitBreakerManager) GetCircuitBreaker(endpoint string) *CircuitBreaker {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	if breaker, exists := cbm.breakers[endpoint]; exists {
		return breaker
	}

	// Create new circuit breaker for this endpoint
	breaker := NewCircuitBreaker(cbm.config)
	cbm.breakers[endpoint] = breaker
	return breaker
}

// GetAllStats returns statistics for all circuit breakers.
func (cbm *CircuitBreakerManager) GetAllStats() map[string]CircuitBreakerStats {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	stats := make(map[string]CircuitBreakerStats)
	for endpoint, breaker := range cbm.breakers {
		stats[endpoint] = breaker.GetStats()
	}
	return stats
}

// ResetAll resets all circuit breakers.
func (cbm *CircuitBreakerManager) ResetAll() {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	for _, breaker := range cbm.breakers {
		breaker.Reset()
	}
}

// ResetEndpoint resets the circuit breaker for a specific endpoint.
func (cbm *CircuitBreakerManager) ResetEndpoint(endpoint string) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	if breaker, exists := cbm.breakers[endpoint]; exists {
		breaker.Reset()
	}
}

// CleanupStaleBreakers removes circuit breakers that haven't been used recently.
func (cbm *CircuitBreakerManager) CleanupStaleBreakers(staleDuration time.Duration) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	cutoff := time.Now().Add(-staleDuration)
	for endpoint, breaker := range cbm.breakers {
		stats := breaker.GetStats()
		// Remove breakers that are closed and haven't failed recently
		// Only remove if there was a failure and it's old enough
		if stats.State == CircuitBreakerClosed &&
			!stats.LastFailureTime.IsZero() &&
			stats.LastFailureTime.Before(cutoff) {
			delete(cbm.breakers, endpoint)
		}
	}
}
