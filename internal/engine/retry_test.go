package engine

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestNewRetryableExecutor(t *testing.T) {
	config := DefaultRetryConfig()
	re := NewRetryableExecutor(config)

	if re == nil {
		t.Fatal("Expected retryable executor to be created")
	}
}

func TestRetryableExecutorSuccess(t *testing.T) {
	config := DefaultRetryConfig()
	re := NewRetryableExecutor(config)

	attempts := 0
	fn := func() error {
		attempts++
		return nil // Success on first try
	}

	ctx := context.Background()
	err := re.Execute(ctx, fn)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got: %d", attempts)
	}
}

func TestRetryableExecutorRetryOnFailure(t *testing.T) {
	config := RetryConfig{
		MaxRetries:      2,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        10 * time.Millisecond,
		BackoffFactor:   2.0,
		JitterPercent:   0,
		RetryableErrors: []string{"connection refused"},
	}
	re := NewRetryableExecutor(config)

	attempts := 0
	fn := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("connection refused")
		}
		return nil // Success on third try
	}

	ctx := context.Background()
	start := time.Now()
	err := re.Execute(ctx, fn)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got: %d", attempts)
	}
	// Should have some delay due to retries
	if elapsed < time.Millisecond {
		t.Errorf("Expected some delay due to retries, got: %v", elapsed)
	}
}

func TestRetryableExecutorMaxRetriesExceeded(t *testing.T) {
	config := RetryConfig{
		MaxRetries:      2,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        10 * time.Millisecond,
		BackoffFactor:   2.0,
		JitterPercent:   0,
		RetryableErrors: []string{"connection refused"},
	}
	re := NewRetryableExecutor(config)

	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("connection refused")
	}

	ctx := context.Background()
	err := re.Execute(ctx, fn)

	if err == nil {
		t.Error("Expected error after max retries exceeded")
	}
	if !strings.Contains(err.Error(), "max retries") {
		t.Errorf("Expected max retries error, got: %v", err)
	}
	if attempts != 3 { // 1 initial + 2 retries
		t.Errorf("Expected 3 attempts, got: %d", attempts)
	}
}

func TestRetryableExecutorNonRetryableError(t *testing.T) {
	config := DefaultRetryConfig()
	re := NewRetryableExecutor(config)

	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("non-retryable error")
	}

	ctx := context.Background()
	err := re.Execute(ctx, fn)

	if err == nil {
		t.Error("Expected error to be returned")
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt for non-retryable error, got: %d", attempts)
	}
}

func TestRetryableExecutorContextCancellation(t *testing.T) {
	config := RetryConfig{
		MaxRetries:      5,
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		BackoffFactor:   2.0,
		JitterPercent:   0,
		RetryableErrors: []string{"connection refused"},
	}
	re := NewRetryableExecutor(config)

	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("connection refused")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := re.Execute(ctx, fn)

	if err != context.DeadlineExceeded {
		t.Errorf("Expected context deadline exceeded, got: %v", err)
	}
	// Should only attempt once before context timeout
	if attempts > 2 {
		t.Errorf("Expected few attempts due to context timeout, got: %d", attempts)
	}
}

func TestRetryableExecutorWithCallback(t *testing.T) {
	config := RetryConfig{
		MaxRetries:      2,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        10 * time.Millisecond,
		BackoffFactor:   2.0,
		JitterPercent:   0,
		RetryableErrors: []string{"connection refused"},
	}
	re := NewRetryableExecutor(config)

	attempts := 0
	retryCallbacks := 0
	fn := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("connection refused")
		}
		return nil
	}

	onRetry := func(attempt int, err error) {
		retryCallbacks++
		if attempt < 1 {
			t.Errorf("Expected attempt >= 1, got: %d", attempt)
		}
		if err == nil {
			t.Error("Expected error in retry callback")
		}
	}

	ctx := context.Background()
	err := re.ExecuteWithCallback(ctx, fn, onRetry)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if retryCallbacks != 2 {
		t.Errorf("Expected 2 retry callbacks, got: %d", retryCallbacks)
	}
}

func TestIsRetryableError(t *testing.T) {
	config := DefaultRetryConfig()
	re := NewRetryableExecutor(config)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "timeout error",
			err:      errors.New("timeout"),
			expected: true,
		},
		{
			name:     "service unavailable",
			err:      errors.New("service unavailable"),
			expected: true,
		},
		{
			name:     "non-retryable error",
			err:      errors.New("invalid input"),
			expected: false,
		},
		{
			name:     "HTTP 429 error",
			err:      NewHTTPError(http.StatusTooManyRequests, "Rate limited"),
			expected: true,
		},
		{
			name:     "HTTP 500 error",
			err:      NewHTTPError(http.StatusInternalServerError, "Internal error"),
			expected: true,
		},
		{
			name:     "HTTP 400 error",
			err:      NewHTTPError(http.StatusBadRequest, "Bad request"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := re.isRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("isRetryableError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsRetryableErrorNetworkErrors(t *testing.T) {
	config := DefaultRetryConfig()
	re := NewRetryableExecutor(config)

	// Test timeout error
	timeoutErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: &mockTimeoutError{},
	}
	if !re.isRetryableError(timeoutErr) {
		t.Error("Expected timeout error to be retryable")
	}

	// Note: Temporary() check removed as it's deprecated in Go 1.18+

	// Test syscall errors
	errno := syscall.ECONNREFUSED
	connRefusedErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: &errno,
	}
	if !re.isRetryableError(connRefusedErr) {
		t.Error("Expected connection refused error to be retryable")
	}
}

func TestCalculateDelay(t *testing.T) {
	config := RetryConfig{
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      1 * time.Second,
		BackoffFactor: 2.0,
		JitterPercent: 0, // No jitter for predictable testing
	}
	re := NewRetryableExecutor(config)

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},   // 100ms * 2^0 = 100ms
		{1, 200 * time.Millisecond},   // 100ms * 2^1 = 200ms
		{2, 400 * time.Millisecond},   // 100ms * 2^2 = 400ms
		{3, 800 * time.Millisecond},   // 100ms * 2^3 = 800ms
		{4, 1000 * time.Millisecond},  // 100ms * 2^4 = 1600ms, capped at 1000ms
		{10, 1000 * time.Millisecond}, // Should be capped at max delay
	}

	for _, tt := range tests {
		delay := re.calculateDelay(tt.attempt)
		if delay != tt.expected {
			t.Errorf("calculateDelay(%d) = %v, want %v", tt.attempt, delay, tt.expected)
		}
	}
}

func TestCalculateDelayWithJitter(t *testing.T) {
	config := RetryConfig{
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      1 * time.Second,
		BackoffFactor: 2.0,
		JitterPercent: 0.1, // 10% jitter
	}
	re := NewRetryableExecutor(config)

	// Calculate delay multiple times to test jitter
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = re.calculateDelay(0)
	}

	// All delays should be around 100ms but vary due to jitter
	baseDelay := 100 * time.Millisecond
	minExpected := time.Duration(float64(baseDelay) * 0.9) // 90ms
	maxExpected := time.Duration(float64(baseDelay) * 1.1) // 110ms

	for i, delay := range delays {
		if delay < minExpected || delay > maxExpected {
			t.Errorf("Delay %d (%v) outside expected range [%v, %v]", i, delay, minExpected, maxExpected)
		}
	}
}

func TestHTTPError(t *testing.T) {
	err := NewHTTPError(500, "Internal Server Error")

	expectedMsg := "HTTP 500: Internal Server Error"
	if err.Error() != expectedMsg {
		t.Errorf("HTTPError.Error() = %v, want %v", err.Error(), expectedMsg)
	}

	if err.StatusCode != 500 {
		t.Errorf("HTTPError.StatusCode = %v, want %v", err.StatusCode, 500)
	}
}

func TestRetryStatsCollector(t *testing.T) {
	collector := NewRetryStatsCollector()

	// Initial stats should be empty
	stats := collector.GetStats()
	if stats.TotalAttempts != 0 {
		t.Errorf("Expected 0 total attempts, got %d", stats.TotalAttempts)
	}

	// Record some attempts
	collector.RecordAttempt(100*time.Millisecond, false)
	collector.RecordAttempt(200*time.Millisecond, true)
	collector.RecordAttempt(300*time.Millisecond, false)

	stats = collector.GetStats()
	if stats.TotalAttempts != 3 {
		t.Errorf("Expected 3 total attempts, got %d", stats.TotalAttempts)
	}
	if stats.SuccessfulRetries != 1 {
		t.Errorf("Expected 1 successful retry, got %d", stats.SuccessfulRetries)
	}
	if stats.FailedRetries != 2 {
		t.Errorf("Expected 2 failed retries, got %d", stats.FailedRetries)
	}
	if stats.MaxDelay != 300*time.Millisecond {
		t.Errorf("Expected max delay 300ms, got %v", stats.MaxDelay)
	}
	if stats.AverageDelay != 200*time.Millisecond {
		t.Errorf("Expected average delay 200ms, got %v", stats.AverageDelay)
	}
	if stats.LastRetryTime.IsZero() {
		t.Error("Expected last retry time to be set")
	}
}

func TestRetryStatsCollectorReset(t *testing.T) {
	collector := NewRetryStatsCollector()

	// Record some data
	collector.RecordAttempt(100*time.Millisecond, true)
	collector.RecordAttempt(200*time.Millisecond, false)

	// Reset
	collector.Reset()

	// Stats should be cleared
	stats := collector.GetStats()
	if stats.TotalAttempts != 0 {
		t.Errorf("Expected 0 total attempts after reset, got %d", stats.TotalAttempts)
	}
	if stats.MaxDelay != 0 {
		t.Errorf("Expected 0 max delay after reset, got %v", stats.MaxDelay)
	}
}

func TestResilientExecutor(t *testing.T) {
	cbConfig := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxRequests:      2,
	}
	retryConfig := RetryConfig{
		MaxRetries:      2,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        10 * time.Millisecond,
		BackoffFactor:   2.0,
		JitterPercent:   0,
		RetryableErrors: []string{"connection refused"},
	}

	re := NewResilientExecutor(cbConfig, retryConfig)

	attempts := 0
	fn := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("connection refused")
		}
		return nil
	}

	ctx := context.Background()
	err := re.Execute(ctx, fn)

	if err != nil {
		t.Errorf("Expected no error with resilient executor, got: %v", err)
	}

	// Check stats
	cbStats := re.GetCircuitBreakerStats()
	if cbStats.State != CircuitBreakerClosed {
		t.Errorf("Expected circuit breaker to be closed, got %v", cbStats.State)
	}

	retryStats := re.GetRetryStats()
	if retryStats.SuccessfulRetries == 0 {
		t.Error("Expected some successful retries")
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries <= 0 {
		t.Error("Expected positive max retries")
	}
	if config.InitialDelay <= 0 {
		t.Error("Expected positive initial delay")
	}
	if config.MaxDelay <= config.InitialDelay {
		t.Error("Expected max delay to be greater than initial delay")
	}
	if config.BackoffFactor <= 1.0 {
		t.Error("Expected backoff factor > 1.0")
	}
	if config.JitterPercent < 0 || config.JitterPercent > 1 {
		t.Error("Expected jitter percent between 0 and 1")
	}
	if len(config.RetryableErrors) == 0 {
		t.Error("Expected some retryable error patterns")
	}
}

// Mock types for testing network errors.
type mockTimeoutError struct{}

func (e *mockTimeoutError) Error() string   { return "timeout" }
func (e *mockTimeoutError) Timeout() bool   { return true }
func (e *mockTimeoutError) Temporary() bool { return false }
