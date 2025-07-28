package engine

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"
)

// RetryConfig defines the configuration for retry mechanisms.
type RetryConfig struct {
	MaxRetries      int           `yaml:"max_retries"`      // Maximum number of retry attempts
	InitialDelay    time.Duration `yaml:"initial_delay"`    // Initial delay before first retry
	MaxDelay        time.Duration `yaml:"max_delay"`        // Maximum delay between retries
	BackoffFactor   float64       `yaml:"backoff_factor"`   // Exponential backoff multiplier
	JitterPercent   float64       `yaml:"jitter_percent"`   // Percentage of jitter to add (0-1)
	RetryableErrors []string      `yaml:"retryable_errors"` // Error patterns that are retryable
}

// DefaultRetryConfig returns a sensible default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      10 * time.Second,
		BackoffFactor: 2.0,
		JitterPercent: 0.1, // 10% jitter
		RetryableErrors: []string{
			"connection refused",
			"connection reset",
			"timeout",
			"temporary failure",
			"service unavailable",
			"internal server error",
			"bad gateway",
			"gateway timeout",
			"too many requests",
		},
	}
}

// RetryableExecutor executes functions with retry logic and exponential backoff.
type RetryableExecutor struct {
	config RetryConfig
	rand   *rand.Rand
}

// NewRetryableExecutor creates a new retryable executor with the given configuration.
func NewRetryableExecutor(config RetryConfig) *RetryableExecutor {
	return &RetryableExecutor{
		config: config,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Execute executes a function with retry logic.
func (re *RetryableExecutor) Execute(ctx context.Context, fn func() error) error {
	return re.ExecuteWithCallback(ctx, fn, nil)
}

// ExecuteWithCallback executes a function with retry logic and optional callback for each attempt.
func (re *RetryableExecutor) ExecuteWithCallback(ctx context.Context, fn func() error, onRetry func(attempt int, err error)) error {
	var lastErr error

	for attempt := 0; attempt <= re.config.MaxRetries; attempt++ {
		// Check if context was cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute the function
		lastErr = fn()
		if lastErr == nil {
			return nil // Success
		}

		// Don't retry on the last attempt
		if attempt == re.config.MaxRetries {
			break
		}

		// Check if the error is retryable
		if !re.isRetryableError(lastErr) {
			return lastErr // Non-retryable error
		}

		// Call the retry callback if provided
		if onRetry != nil {
			onRetry(attempt+1, lastErr)
		}

		// Calculate delay for next attempt
		delay := re.calculateDelay(attempt)

		// Wait before retrying
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return fmt.Errorf("max retries (%d) exceeded, last error: %v", re.config.MaxRetries, lastErr)
}

// isRetryableError determines if an error should be retried.
func (re *RetryableExecutor) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := strings.ToLower(err.Error())

	// Check for network errors that are typically transient
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			return true
		}
	}

	// Check for syscall errors
	if opErr, ok := err.(*net.OpError); ok {
		if syscallErr, ok := opErr.Err.(*syscall.Errno); ok {
			switch *syscallErr {
			case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ETIMEDOUT:
				return true
			}
		}
	}

	// Check for HTTP errors that are retryable
	if httpErr, ok := err.(*HTTPError); ok {
		switch httpErr.StatusCode {
		case http.StatusTooManyRequests, // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout:      // 504
			return true
		}
	}

	// Check configured retryable error patterns
	for _, pattern := range re.config.RetryableErrors {
		if strings.Contains(errorStr, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// calculateDelay calculates the delay for the next retry attempt with exponential backoff and jitter.
func (re *RetryableExecutor) calculateDelay(attempt int) time.Duration {
	// Calculate base delay with exponential backoff
	delay := float64(re.config.InitialDelay) * math.Pow(re.config.BackoffFactor, float64(attempt))

	// Apply maximum delay cap
	if delay > float64(re.config.MaxDelay) {
		delay = float64(re.config.MaxDelay)
	}

	// Add jitter to prevent thundering herd
	if re.config.JitterPercent > 0 {
		jitter := delay * re.config.JitterPercent * (re.rand.Float64()*2 - 1) // -jitter to +jitter
		delay += jitter
	}

	// Ensure delay is not negative
	if delay < 0 {
		delay = float64(re.config.InitialDelay)
	}

	return time.Duration(delay)
}

// HTTPError represents an HTTP error with status code.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// NewHTTPError creates a new HTTP error.
func NewHTTPError(statusCode int, message string) *HTTPError {
	return &HTTPError{
		StatusCode: statusCode,
		Message:    message,
	}
}

// RetryStats contains statistics about retry operations.
type RetryStats struct {
	TotalAttempts     int           `json:"total_attempts"`
	SuccessfulRetries int           `json:"successful_retries"`
	FailedRetries     int           `json:"failed_retries"`
	AverageDelay      time.Duration `json:"average_delay"`
	MaxDelay          time.Duration `json:"max_delay"`
	LastRetryTime     time.Time     `json:"last_retry_time"`
}

// RetryStatsCollector collects statistics about retry operations.
type RetryStatsCollector struct {
	stats  RetryStats
	delays []time.Duration
	mu     sync.RWMutex
}

// NewRetryStatsCollector creates a new retry statistics collector.
func NewRetryStatsCollector() *RetryStatsCollector {
	return &RetryStatsCollector{
		delays: make([]time.Duration, 0),
	}
}

// RecordAttempt records a retry attempt.
func (rsc *RetryStatsCollector) RecordAttempt(delay time.Duration, success bool) {
	rsc.mu.Lock()
	defer rsc.mu.Unlock()

	rsc.stats.TotalAttempts++
	rsc.stats.LastRetryTime = time.Now()

	if success {
		rsc.stats.SuccessfulRetries++
	} else {
		rsc.stats.FailedRetries++
	}

	// Record delay for averaging
	rsc.delays = append(rsc.delays, delay)
	if delay > rsc.stats.MaxDelay {
		rsc.stats.MaxDelay = delay
	}

	// Calculate average delay
	var total time.Duration
	for _, d := range rsc.delays {
		total += d
	}
	rsc.stats.AverageDelay = total / time.Duration(len(rsc.delays))

	// Keep only recent delays (last 100) to prevent memory growth
	if len(rsc.delays) > 100 {
		rsc.delays = rsc.delays[len(rsc.delays)-100:]
	}
}

// GetStats returns the current retry statistics.
func (rsc *RetryStatsCollector) GetStats() RetryStats {
	rsc.mu.RLock()
	defer rsc.mu.RUnlock()
	return rsc.stats
}

// Reset resets the retry statistics.
func (rsc *RetryStatsCollector) Reset() {
	rsc.mu.Lock()
	defer rsc.mu.Unlock()

	rsc.stats = RetryStats{}
	rsc.delays = rsc.delays[:0]
}

// ResilientExecutor combines circuit breaker and retry mechanisms for robust execution.
type ResilientExecutor struct {
	circuitBreaker *CircuitBreaker
	retryExecutor  *RetryableExecutor
	statsCollector *RetryStatsCollector
}

// NewResilientExecutor creates a new resilient executor with circuit breaker and retry.
func NewResilientExecutor(cbConfig CircuitBreakerConfig, retryConfig RetryConfig) *ResilientExecutor {
	return &ResilientExecutor{
		circuitBreaker: NewCircuitBreaker(cbConfig),
		retryExecutor:  NewRetryableExecutor(retryConfig),
		statsCollector: NewRetryStatsCollector(),
	}
}

// Execute executes a function with both circuit breaker and retry protection.
func (re *ResilientExecutor) Execute(ctx context.Context, fn func() error) error {
	err := re.circuitBreaker.Call(func() error {
		return re.retryExecutor.ExecuteWithCallback(ctx, fn, func(attempt int, retryErr error) {
			// Record failed retry attempts
			re.statsCollector.RecordAttempt(re.retryExecutor.calculateDelay(attempt-1), false)
		})
	})

	// If the overall execution succeeded after retries, record the success
	if err == nil {
		re.statsCollector.RecordAttempt(0, true)
	}

	return err
}

// GetCircuitBreakerStats returns circuit breaker statistics.
func (re *ResilientExecutor) GetCircuitBreakerStats() CircuitBreakerStats {
	return re.circuitBreaker.GetStats()
}

// GetRetryStats returns retry statistics.
func (re *ResilientExecutor) GetRetryStats() RetryStats {
	return re.statsCollector.GetStats()
}

// Reset resets both circuit breaker and retry statistics.
func (re *ResilientExecutor) Reset() {
	re.circuitBreaker.Reset()
	re.statsCollector.Reset()
}
