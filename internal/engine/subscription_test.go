package engine

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dangazineu/tako/internal/config"
)

func TestNewSubscriptionEvaluator(t *testing.T) {
	se, err := NewSubscriptionEvaluator()
	if err != nil {
		t.Fatalf("Failed to create subscription evaluator: %v", err)
	}
	if se == nil {
		t.Fatal("Expected non-nil subscription evaluator")
	}
	if se.celEnv == nil {
		t.Fatal("Expected non-nil CEL environment")
	}
	if se.costLimit != 1000000 {
		t.Errorf("Expected cost limit 1000000, got %d", se.costLimit)
	}
	if se.cacheLimit != 1000 {
		t.Errorf("Expected cache limit 1000, got %d", se.cacheLimit)
	}

	// Verify cache is initialized
	size, limit := se.GetCacheStats()
	if size != 0 {
		t.Errorf("Expected initial cache size 0, got %d", size)
	}
	if limit != 1000 {
		t.Errorf("Expected cache limit 1000, got %d", limit)
	}
}

func TestSubscriptionEvaluator_EvaluateSubscription(t *testing.T) {
	se, err := NewSubscriptionEvaluator()
	if err != nil {
		t.Fatalf("Failed to create subscription evaluator: %v", err)
	}

	event := Event{
		Type:          "library_built",
		SchemaVersion: "1.0.0",
		Payload: map[string]interface{}{
			"version": "2.1.0",
			"status":  "success",
		},
		Source:    "test-org/library",
		Timestamp: time.Now().Unix(),
	}

	tests := []struct {
		name         string
		subscription config.Subscription
		event        Event
		want         bool
		expectError  bool
	}{
		{
			name: "exact event type match",
			subscription: config.Subscription{
				Events:   []string{"library_built"},
				Workflow: "update",
			},
			event: event,
			want:  true,
		},
		{
			name: "multiple event types - match",
			subscription: config.Subscription{
				Events:   []string{"library_updated", "library_built", "library_deployed"},
				Workflow: "update",
			},
			event: event,
			want:  true,
		},
		{
			name: "event type mismatch",
			subscription: config.Subscription{
				Events:   []string{"library_deployed"},
				Workflow: "deploy",
			},
			event: event,
			want:  false,
		},
		{
			name: "schema version exact match",
			subscription: config.Subscription{
				Events:        []string{"library_built"},
				SchemaVersion: "1.0.0",
				Workflow:      "update",
			},
			event: event,
			want:  true,
		},
		{
			name: "schema version caret range match",
			subscription: config.Subscription{
				Events:        []string{"library_built"},
				SchemaVersion: "^1.0.0",
				Workflow:      "update",
			},
			event: event,
			want:  true,
		},
		{
			name: "schema version incompatible",
			subscription: config.Subscription{
				Events:        []string{"library_built"},
				SchemaVersion: "2.0.0",
				Workflow:      "update",
			},
			event: event,
			want:  false,
		},
		{
			name: "CEL filter match",
			subscription: config.Subscription{
				Events:   []string{"library_built"},
				Filters:  []string{"payload.status == 'success'"},
				Workflow: "update",
			},
			event: event,
			want:  true,
		},
		{
			name: "CEL filter no match",
			subscription: config.Subscription{
				Events:   []string{"library_built"},
				Filters:  []string{"payload.status == 'failed'"},
				Workflow: "update",
			},
			event: event,
			want:  false,
		},
		{
			name: "multiple CEL filters - all match",
			subscription: config.Subscription{
				Events:   []string{"library_built"},
				Filters:  []string{"payload.status == 'success'", "event_type == 'library_built'"},
				Workflow: "update",
			},
			event: event,
			want:  true,
		},
		{
			name: "multiple CEL filters - one fails",
			subscription: config.Subscription{
				Events:   []string{"library_built"},
				Filters:  []string{"payload.status == 'success'", "payload.version == '1.0.0'"},
				Workflow: "update",
			},
			event: event,
			want:  false,
		},
		{
			name: "invalid CEL filter",
			subscription: config.Subscription{
				Events:   []string{"library_built"},
				Filters:  []string{"invalid syntax ]["},
				Workflow: "update",
			},
			event:       event,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := se.EvaluateSubscription(tt.subscription, tt.event)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("EvaluateSubscription() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscriptionEvaluator_CheckSchemaCompatibility(t *testing.T) {
	se, err := NewSubscriptionEvaluator()
	if err != nil {
		t.Fatalf("Failed to create subscription evaluator: %v", err)
	}

	tests := []struct {
		name              string
		eventVersion      string
		subscriptionRange string
		want              bool
		expectError       bool
	}{
		{
			name:              "exact version match",
			eventVersion:      "1.0.0",
			subscriptionRange: "1.0.0",
			want:              true,
		},
		{
			name:              "exact version mismatch",
			eventVersion:      "1.0.0",
			subscriptionRange: "2.0.0",
			want:              false,
		},
		{
			name:              "caret range compatible",
			eventVersion:      "1.2.3",
			subscriptionRange: "^1.0.0",
			want:              true,
		},
		{
			name:              "caret range incompatible major",
			eventVersion:      "2.0.0",
			subscriptionRange: "^1.0.0",
			want:              false,
		},
		{
			name:              "tilde range compatible",
			eventVersion:      "1.0.5",
			subscriptionRange: "~1.0.0",
			want:              true,
		},
		{
			name:              "tilde range incompatible minor",
			eventVersion:      "1.1.0",
			subscriptionRange: "~1.0.0",
			want:              false,
		},
		{
			name:              "greater than or equal - equal",
			eventVersion:      "1.0.0",
			subscriptionRange: ">=1.0.0",
			want:              true,
		},
		{
			name:              "greater than or equal - greater",
			eventVersion:      "1.0.1",
			subscriptionRange: ">=1.0.0",
			want:              true,
		},
		{
			name:              "greater than or equal - less",
			eventVersion:      "0.9.0",
			subscriptionRange: ">=1.0.0",
			want:              false,
		},
		{
			name:              "empty event version - compatible",
			eventVersion:      "",
			subscriptionRange: "1.0.0",
			want:              true,
		},
		{
			name:              "empty subscription range - compatible",
			eventVersion:      "1.0.0",
			subscriptionRange: "",
			want:              true,
		},
		{
			name:              "both empty - compatible",
			eventVersion:      "",
			subscriptionRange: "",
			want:              true,
		},
		{
			name:              "invalid event version",
			eventVersion:      "invalid",
			subscriptionRange: "1.0.0",
			expectError:       true,
		},
		{
			name:              "invalid subscription range",
			eventVersion:      "1.0.0",
			subscriptionRange: "invalid",
			expectError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := se.CheckSchemaCompatibility(tt.eventVersion, tt.subscriptionRange)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("CheckSchemaCompatibility() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscriptionEvaluator_ProcessEventPayload(t *testing.T) {
	se, err := NewSubscriptionEvaluator()
	if err != nil {
		t.Fatalf("Failed to create subscription evaluator: %v", err)
	}

	payload := map[string]interface{}{
		"version": "2.1.0",
		"status":  "success",
		"tags":    []string{"latest", "stable"},
	}

	tests := []struct {
		name         string
		payload      map[string]interface{}
		subscription config.Subscription
		want         map[string]string
		expectError  bool
	}{
		{
			name:    "simple template substitution",
			payload: payload,
			subscription: config.Subscription{
				Inputs: map[string]string{
					"version": "{{ .payload.version }}",
					"status":  "{{ .payload.status }}",
				},
			},
			want: map[string]string{
				"version": "2.1.0",
				"status":  "success",
			},
		},
		{
			name:    "literal input values",
			payload: payload,
			subscription: config.Subscription{
				Inputs: map[string]string{
					"environment": "production",
					"action":      "deploy",
				},
			},
			want: map[string]string{
				"environment": "production",
				"action":      "deploy",
			},
		},
		{
			name:    "mixed template and literal",
			payload: payload,
			subscription: config.Subscription{
				Inputs: map[string]string{
					"version":     "{{ .payload.version }}",
					"environment": "staging",
				},
			},
			want: map[string]string{
				"version":     "2.1.0",
				"environment": "staging",
			},
		},
		{
			name:    "nonexistent payload field",
			payload: payload,
			subscription: config.Subscription{
				Inputs: map[string]string{
					"missing": "{{ .payload.nonexistent }}",
				},
			},
			expectError: true,
		},
		{
			name:    "empty inputs",
			payload: payload,
			subscription: config.Subscription{
				Inputs: map[string]string{},
			},
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := se.ProcessEventPayload(tt.payload, tt.subscription)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("ProcessEventPayload() result length = %v, want %v", len(got), len(tt.want))
				return
			}

			for key, expectedValue := range tt.want {
				if actualValue, exists := got[key]; !exists {
					t.Errorf("ProcessEventPayload() missing key %s", key)
				} else if actualValue != expectedValue {
					t.Errorf("ProcessEventPayload() key %s = %v, want %v", key, actualValue, expectedValue)
				}
			}
		})
	}
}

func TestParseSemVer(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		want        SemVer
		expectError bool
	}{
		{
			name:    "valid version",
			version: "1.2.3",
			want:    SemVer{Major: 1, Minor: 2, Patch: 3},
		},
		{
			name:    "version with zeros",
			version: "0.0.0",
			want:    SemVer{Major: 0, Minor: 0, Patch: 0},
		},
		{
			name:    "large version numbers",
			version: "10.20.30",
			want:    SemVer{Major: 10, Minor: 20, Patch: 30},
		},
		{
			name:        "invalid format - missing patch",
			version:     "1.2",
			expectError: true,
		},
		{
			name:        "invalid format - extra component",
			version:     "1.2.3.4",
			expectError: true,
		},
		{
			name:        "invalid format - non-numeric",
			version:     "1.2.x",
			expectError: true,
		},
		{
			name:        "empty string",
			version:     "",
			expectError: true,
		},
		{
			name:        "invalid format - with v prefix",
			version:     "v1.2.3",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSemVer(tt.version)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("parseSemVer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateVersionRange(t *testing.T) {
	tests := []struct {
		name        string
		version     SemVer
		rangeSpec   string
		want        bool
		expectError bool
	}{
		{
			name:      "exact match",
			version:   SemVer{1, 2, 3},
			rangeSpec: "1.2.3",
			want:      true,
		},
		{
			name:      "exact no match",
			version:   SemVer{1, 2, 3},
			rangeSpec: "1.2.4",
			want:      false,
		},
		{
			name:      "caret range - same version",
			version:   SemVer{1, 2, 3},
			rangeSpec: "^1.2.3",
			want:      true,
		},
		{
			name:      "caret range - higher minor",
			version:   SemVer{1, 3, 0},
			rangeSpec: "^1.2.3",
			want:      true,
		},
		{
			name:      "caret range - higher patch",
			version:   SemVer{1, 2, 5},
			rangeSpec: "^1.2.3",
			want:      true,
		},
		{
			name:      "caret range - incompatible major",
			version:   SemVer{2, 0, 0},
			rangeSpec: "^1.2.3",
			want:      false,
		},
		{
			name:      "caret range - lower minor",
			version:   SemVer{1, 1, 0},
			rangeSpec: "^1.2.3",
			want:      false,
		},
		{
			name:      "tilde range - same version",
			version:   SemVer{1, 2, 3},
			rangeSpec: "~1.2.3",
			want:      true,
		},
		{
			name:      "tilde range - higher patch",
			version:   SemVer{1, 2, 5},
			rangeSpec: "~1.2.3",
			want:      true,
		},
		{
			name:      "tilde range - incompatible minor",
			version:   SemVer{1, 3, 0},
			rangeSpec: "~1.2.3",
			want:      false,
		},
		{
			name:      "greater than or equal - equal",
			version:   SemVer{1, 2, 3},
			rangeSpec: ">=1.2.3",
			want:      true,
		},
		{
			name:      "greater than or equal - greater",
			version:   SemVer{1, 2, 4},
			rangeSpec: ">=1.2.3",
			want:      true,
		},
		{
			name:      "greater than or equal - less",
			version:   SemVer{1, 2, 2},
			rangeSpec: ">=1.2.3",
			want:      false,
		},
		{
			name:      "greater than - greater",
			version:   SemVer{1, 2, 4},
			rangeSpec: ">1.2.3",
			want:      true,
		},
		{
			name:      "greater than - equal",
			version:   SemVer{1, 2, 3},
			rangeSpec: ">1.2.3",
			want:      false,
		},
		{
			name:        "invalid range format",
			version:     SemVer{1, 2, 3},
			rangeSpec:   "invalid",
			expectError: true,
		},
		{
			name:        "unsupported operator",
			version:     SemVer{1, 2, 3},
			rangeSpec:   "!=1.2.3",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateVersionRange(tt.version, tt.rangeSpec)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("evaluateVersionRange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		v1   SemVer
		v2   SemVer
		want int
	}{
		{
			name: "equal versions",
			v1:   SemVer{1, 2, 3},
			v2:   SemVer{1, 2, 3},
			want: 0,
		},
		{
			name: "v1 major > v2 major",
			v1:   SemVer{2, 0, 0},
			v2:   SemVer{1, 9, 9},
			want: 1,
		},
		{
			name: "v1 major < v2 major",
			v1:   SemVer{1, 9, 9},
			v2:   SemVer{2, 0, 0},
			want: -1,
		},
		{
			name: "v1 minor > v2 minor",
			v1:   SemVer{1, 3, 0},
			v2:   SemVer{1, 2, 9},
			want: 1,
		},
		{
			name: "v1 minor < v2 minor",
			v1:   SemVer{1, 2, 9},
			v2:   SemVer{1, 3, 0},
			want: -1,
		},
		{
			name: "v1 patch > v2 patch",
			v1:   SemVer{1, 2, 4},
			v2:   SemVer{1, 2, 3},
			want: 1,
		},
		{
			name: "v1 patch < v2 patch",
			v1:   SemVer{1, 2, 3},
			v2:   SemVer{1, 2, 4},
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscriptionEvaluator_CELCaching(t *testing.T) {
	se, err := NewSubscriptionEvaluator()
	if err != nil {
		t.Fatalf("Failed to create subscription evaluator: %v", err)
	}

	event := Event{
		Type:          "library_built",
		SchemaVersion: "1.0.0",
		Payload: map[string]interface{}{
			"version": "2.1.0",
			"status":  "success",
		},
		Source:    "test-org/library",
		Timestamp: time.Now().Unix(),
	}

	// Test basic caching functionality
	t.Run("cache stores and retrieves compiled programs", func(t *testing.T) {
		// Clear cache to start fresh
		se.ClearCache()

		size, _ := se.GetCacheStats()
		if size != 0 {
			t.Errorf("Expected cache size 0 after clear, got %d", size)
		}

		subscription := config.Subscription{
			Events:   []string{"library_built"},
			Filters:  []string{"payload.status == 'success'"},
			Workflow: "update",
		}

		// First evaluation should compile and cache
		result1, err := se.EvaluateSubscription(subscription, event)
		if err != nil {
			t.Fatalf("First evaluation failed: %v", err)
		}
		if !result1 {
			t.Errorf("Expected first evaluation to return true")
		}

		// Cache should now contain one program
		size, _ = se.GetCacheStats()
		if size != 1 {
			t.Errorf("Expected cache size 1 after first evaluation, got %d", size)
		}

		// Second evaluation should use cached program
		result2, err := se.EvaluateSubscription(subscription, event)
		if err != nil {
			t.Fatalf("Second evaluation failed: %v", err)
		}
		if !result2 {
			t.Errorf("Expected second evaluation to return true")
		}

		// Cache size should remain the same
		size, _ = se.GetCacheStats()
		if size != 1 {
			t.Errorf("Expected cache size 1 after second evaluation, got %d", size)
		}
	})

	t.Run("different expressions create separate cache entries", func(t *testing.T) {
		se.ClearCache()

		subscriptions := []config.Subscription{
			{
				Events:   []string{"library_built"},
				Filters:  []string{"payload.status == 'success'"},
				Workflow: "update1",
			},
			{
				Events:   []string{"library_built"},
				Filters:  []string{"payload.version == '2.1.0'"},
				Workflow: "update2",
			},
			{
				Events:   []string{"library_built"},
				Filters:  []string{"event_type == 'library_built'"},
				Workflow: "update3",
			},
		}

		// Evaluate each subscription
		for i, sub := range subscriptions {
			result, err := se.EvaluateSubscription(sub, event)
			if err != nil {
				t.Fatalf("Evaluation %d failed: %v", i, err)
			}
			if !result {
				t.Errorf("Expected evaluation %d to return true", i)
			}

			// Cache size should increase
			size, _ := se.GetCacheStats()
			expectedSize := int64(i + 1)
			if size != expectedSize {
				t.Errorf("Expected cache size %d after evaluation %d, got %d", expectedSize, i, size)
			}
		}
	})

	t.Run("cache eviction when limit is reached", func(t *testing.T) {
		// Create a new evaluator with a small cache limit for testing
		seSmall := &SubscriptionEvaluator{
			celEnv:     se.celEnv,
			costLimit:  1000000,
			cacheLimit: 2, // Very small cache for testing
		}

		subscriptions := []config.Subscription{
			{
				Events:   []string{"library_built"},
				Filters:  []string{"payload.status == 'success'"},
				Workflow: "update1",
			},
			{
				Events:   []string{"library_built"},
				Filters:  []string{"payload.version == '2.1.0'"},
				Workflow: "update2",
			},
			{
				Events:   []string{"library_built"},
				Filters:  []string{"event_type == 'library_built'"},
				Workflow: "update3",
			},
		}

		// Fill cache to limit
		for i := 0; i < 2; i++ {
			_, err := seSmall.EvaluateSubscription(subscriptions[i], event)
			if err != nil {
				t.Fatalf("Evaluation %d failed: %v", i, err)
			}
		}

		size, _ := seSmall.GetCacheStats()
		if size != 2 {
			t.Errorf("Expected cache size 2 after filling, got %d", size)
		}

		// Adding one more should trigger eviction
		_, err := seSmall.EvaluateSubscription(subscriptions[2], event)
		if err != nil {
			t.Fatalf("Evaluation after cache full failed: %v", err)
		}

		// Cache should be cleared and contain only the new entry
		size, _ = seSmall.GetCacheStats()
		if size != 1 {
			t.Errorf("Expected cache size 1 after eviction, got %d", size)
		}
	})

	t.Run("concurrent access to cache is thread-safe", func(t *testing.T) {
		se.ClearCache()

		numGoroutines := 10
		numEvaluationsPerGoroutine := 50

		subscription := config.Subscription{
			Events:   []string{"library_built"},
			Filters:  []string{"payload.status == 'success' && payload.version == '2.1.0'"},
			Workflow: "concurrent_test",
		}

		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines*numEvaluationsPerGoroutine)

		// Launch multiple goroutines that evaluate the same subscription
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < numEvaluationsPerGoroutine; j++ {
					result, err := se.EvaluateSubscription(subscription, event)
					if err != nil {
						errors <- err
						return
					}
					if !result {
						errors <- fmt.Errorf("goroutine %d evaluation %d: expected true result", goroutineID, j)
						return
					}
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for any errors
		for err := range errors {
			t.Errorf("Concurrent evaluation error: %v", err)
		}

		// Cache should contain exactly one entry (the shared expression)
		size, _ := se.GetCacheStats()
		if size != 1 {
			t.Errorf("Expected cache size 1 after concurrent evaluations, got %d", size)
		}
	})

	t.Run("cache clear works correctly", func(t *testing.T) {
		// Add some entries to cache
		subscriptions := []config.Subscription{
			{
				Events:   []string{"library_built"},
				Filters:  []string{"payload.status == 'success'"},
				Workflow: "update1",
			},
			{
				Events:   []string{"library_built"},
				Filters:  []string{"payload.version == '2.1.0'"},
				Workflow: "update2",
			},
		}

		for _, sub := range subscriptions {
			_, err := se.EvaluateSubscription(sub, event)
			if err != nil {
				t.Fatalf("Failed to populate cache: %v", err)
			}
		}

		size, _ := se.GetCacheStats()
		if size == 0 {
			t.Errorf("Expected cache to have entries before clear")
		}

		// Clear cache
		se.ClearCache()

		// Verify cache is empty
		size, _ = se.GetCacheStats()
		if size != 0 {
			t.Errorf("Expected cache size 0 after clear, got %d", size)
		}
	})
}

func TestSubscriptionEvaluator_CELCachingPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	se, err := NewSubscriptionEvaluator()
	if err != nil {
		t.Fatalf("Failed to create subscription evaluator: %v", err)
	}

	event := Event{
		Type:          "library_built",
		SchemaVersion: "1.0.0",
		Payload: map[string]interface{}{
			"version":     "2.1.0",
			"status":      "success",
			"buildNumber": 12345,
		},
		Source:    "test-org/library",
		Timestamp: time.Now().Unix(),
	}

	subscription := config.Subscription{
		Events:   []string{"library_built"},
		Filters:  []string{"payload.status == 'success' && payload.buildNumber > 10000"},
		Workflow: "performance_test",
	}

	numIterations := 1000

	// Measure performance with caching
	se.ClearCache()
	start := time.Now()
	for i := 0; i < numIterations; i++ {
		result, err := se.EvaluateSubscription(subscription, event)
		if err != nil {
			t.Fatalf("Cached evaluation failed: %v", err)
		}
		if !result {
			t.Errorf("Expected evaluation to return true")
		}
	}
	cachedDuration := time.Since(start)

	// Measure performance without caching (clear cache before each evaluation)
	start = time.Now()
	for i := 0; i < numIterations; i++ {
		se.ClearCache() // Force recompilation each time
		result, err := se.EvaluateSubscription(subscription, event)
		if err != nil {
			t.Fatalf("Non-cached evaluation failed: %v", err)
		}
		if !result {
			t.Errorf("Expected evaluation to return true")
		}
	}
	nonCachedDuration := time.Since(start)

	t.Logf("Cached evaluations: %d iterations in %v (%.2f µs per evaluation)",
		numIterations, cachedDuration, float64(cachedDuration.Nanoseconds())/float64(numIterations)/1000)
	t.Logf("Non-cached evaluations: %d iterations in %v (%.2f µs per evaluation)",
		numIterations, nonCachedDuration, float64(nonCachedDuration.Nanoseconds())/float64(numIterations)/1000)

	// Caching should provide significant performance improvement
	if cachedDuration >= nonCachedDuration {
		t.Errorf("Expected caching to improve performance, but cached (%v) >= non-cached (%v)",
			cachedDuration, nonCachedDuration)
	}

	// Log the performance improvement ratio
	improvement := float64(nonCachedDuration) / float64(cachedDuration)
	t.Logf("Performance improvement: %.1fx faster with caching", improvement)

	// We expect at least 2x improvement with caching
	if improvement < 2.0 {
		t.Logf("Warning: Performance improvement (%.1fx) is less than expected (2.0x)", improvement)
	}
}

// TestCheckSchemaCompatibilityDetailed tests the enhanced schema compatibility validation with detailed error reporting.
func TestCheckSchemaCompatibilityDetailed(t *testing.T) {
	se, err := NewSubscriptionEvaluator()
	if err != nil {
		t.Fatalf("Failed to create subscription evaluator: %v", err)
	}

	tests := []struct {
		name              string
		eventVersion      string
		subscriptionRange string
		expectCompatible  bool
		expectReason      string
		expectDetails     string
	}{
		{
			name:              "no event version",
			eventVersion:      "",
			subscriptionRange: "1.0.0",
			expectCompatible:  true,
			expectReason:      "Event has no schema version specified (backward compatibility)",
			expectDetails:     "Events without schema versions are accepted for backward compatibility",
		},
		{
			name:              "no subscription range",
			eventVersion:      "1.0.0",
			subscriptionRange: "",
			expectCompatible:  true,
			expectReason:      "Subscription accepts any schema version",
			expectDetails:     "No version constraint specified in subscription",
		},
		{
			name:              "invalid event version",
			eventVersion:      "invalid",
			subscriptionRange: "1.0.0",
			expectCompatible:  false,
			expectReason:      "Invalid event schema version 'invalid'",
			expectDetails:     "Event schema version must follow semantic versioning (major.minor.patch)",
		},
		{
			name:              "exact version match",
			eventVersion:      "1.2.3",
			subscriptionRange: "1.2.3",
			expectCompatible:  true,
			expectReason:      "Event version 1.2.3 exactly matches required version 1.2.3",
			expectDetails:     "Exact version match provides strongest compatibility guarantee",
		},
		{
			name:              "exact version mismatch",
			eventVersion:      "1.2.4",
			subscriptionRange: "1.2.3",
			expectCompatible:  false,
			expectReason:      "Event version 1.2.4 does not match required version 1.2.3",
			expectDetails:     "Exact version constraints require perfect match for compatibility",
		},
		{
			name:              "caret range compatible",
			eventVersion:      "1.3.0",
			subscriptionRange: "^1.2.0",
			expectCompatible:  true,
			expectReason:      "Event version 1.3.0 is compatible with caret range ^1.2.0",
			expectDetails:     "Caret range allows minor and patch updates within major version 1",
		},
		{
			name:              "caret range major version mismatch",
			eventVersion:      "2.0.0",
			subscriptionRange: "^1.0.0",
			expectCompatible:  false,
			expectReason:      "Event version 2.0.0 has different major version than range ^1.0.0 (breaking changes)",
			expectDetails:     "Caret ranges reject different major versions due to potential breaking changes",
		},
		{
			name:              "caret range too old",
			eventVersion:      "1.1.0",
			subscriptionRange: "^1.2.0",
			expectCompatible:  false,
			expectReason:      "Event version 1.1.0 is older than minimum version in range ^1.2.0",
			expectDetails:     "Caret ranges require version to be at least the specified version",
		},
		{
			name:              "tilde range compatible",
			eventVersion:      "1.2.5",
			subscriptionRange: "~1.2.3",
			expectCompatible:  true,
			expectReason:      "Event version 1.2.5 is compatible with tilde range ~1.2.3",
			expectDetails:     "Tilde range allows patch updates within version 1.2",
		},
		{
			name:              "tilde range minor version mismatch",
			eventVersion:      "1.3.0",
			subscriptionRange: "~1.2.0",
			expectCompatible:  false,
			expectReason:      "Event version 1.3.0 has different major.minor than range ~1.2.0",
			expectDetails:     "Tilde ranges reject different major or minor versions",
		},
		{
			name:              "tilde range too old patch",
			eventVersion:      "1.2.1",
			subscriptionRange: "~1.2.3",
			expectCompatible:  false,
			expectReason:      "Event version 1.2.1 is older than minimum patch in range ~1.2.3",
			expectDetails:     "Tilde ranges require patch version to be at least the specified version",
		},
		{
			name:              "greater than or equal compatible",
			eventVersion:      "1.5.0",
			subscriptionRange: ">=1.2.0",
			expectCompatible:  true,
			expectReason:      "Event version 1.5.0 satisfies >= constraint >=1.2.0",
			expectDetails:     "Greater-than-or-equal allows any version at or above the specified version",
		},
		{
			name:              "greater than or equal too old",
			eventVersion:      "1.1.0",
			subscriptionRange: ">=1.2.0",
			expectCompatible:  false,
			expectReason:      "Event version 1.1.0 is older than minimum required 1.2.0",
			expectDetails:     "Greater-than-or-equal constraint requires version to be at least the specified version",
		},
		{
			name:              "greater than compatible",
			eventVersion:      "1.5.0",
			subscriptionRange: ">1.2.0",
			expectCompatible:  true,
			expectReason:      "Event version 1.5.0 satisfies > constraint >1.2.0",
			expectDetails:     "Greater-than allows any version newer than the specified version",
		},
		{
			name:              "greater than not newer",
			eventVersion:      "1.2.0",
			subscriptionRange: ">1.2.0",
			expectCompatible:  false,
			expectReason:      "Event version 1.2.0 is not newer than required 1.2.0",
			expectDetails:     "Greater-than constraint requires version to be newer than the specified version",
		},
		{
			name:              "less than or equal compatible",
			eventVersion:      "1.1.0",
			subscriptionRange: "<=1.2.0",
			expectCompatible:  true,
			expectReason:      "Event version 1.1.0 satisfies <= constraint <=1.2.0",
			expectDetails:     "Less-than-or-equal allows any version at or below the specified version",
		},
		{
			name:              "less than or equal too new",
			eventVersion:      "1.3.0",
			subscriptionRange: "<=1.2.0",
			expectCompatible:  false,
			expectReason:      "Event version 1.3.0 is newer than maximum allowed 1.2.0",
			expectDetails:     "Less-than-or-equal constraint requires version to be at most the specified version",
		},
		{
			name:              "less than compatible",
			eventVersion:      "1.1.0",
			subscriptionRange: "<1.2.0",
			expectCompatible:  true,
			expectReason:      "Event version 1.1.0 satisfies < constraint <1.2.0",
			expectDetails:     "Less-than allows any version older than the specified version",
		},
		{
			name:              "less than not older",
			eventVersion:      "1.2.0",
			subscriptionRange: "<1.2.0",
			expectCompatible:  false,
			expectReason:      "Event version 1.2.0 is not older than required 1.2.0",
			expectDetails:     "Less-than constraint requires version to be older than the specified version",
		},
		{
			name:              "invalid range format",
			eventVersion:      "1.0.0",
			subscriptionRange: "invalid",
			expectCompatible:  false,
			expectReason:      "Invalid version range 'invalid'",
			expectDetails:     "Version ranges must follow semantic versioning format (major.minor.patch)",
		},
		{
			name:              "unsupported range format",
			eventVersion:      "1.0.0",
			subscriptionRange: "!=1.0.0",
			expectCompatible:  false,
			expectReason:      "Unsupported version range format: !=1.0.0",
			expectDetails:     "Supported formats: exact (1.0.0), caret (^1.0.0), tilde (~1.0.0), comparison operators (>=1.0.0, >1.0.0, <=1.0.0, <1.0.0)",
		},
		{
			name:              "invalid caret range",
			eventVersion:      "1.0.0",
			subscriptionRange: "^invalid",
			expectCompatible:  false,
			expectReason:      "Invalid caret range '^invalid'",
			expectDetails:     "Caret ranges must follow format ^major.minor.patch",
		},
		{
			name:              "invalid tilde range",
			eventVersion:      "1.0.0",
			subscriptionRange: "~invalid",
			expectCompatible:  false,
			expectReason:      "Invalid tilde range '~invalid'",
			expectDetails:     "Tilde ranges must follow format ~major.minor.patch",
		},
		{
			name:              "invalid >= range",
			eventVersion:      "1.0.0",
			subscriptionRange: ">=invalid",
			expectCompatible:  false,
			expectReason:      "Invalid >= range '>=invalid'",
			expectDetails:     ">= ranges must follow format >=major.minor.patch",
		},
		{
			name:              "invalid > range",
			eventVersion:      "1.0.0",
			subscriptionRange: ">invalid",
			expectCompatible:  false,
			expectReason:      "Invalid > range '>invalid'",
			expectDetails:     "> ranges must follow format >major.minor.patch",
		},
		{
			name:              "invalid <= range",
			eventVersion:      "1.0.0",
			subscriptionRange: "<=invalid",
			expectCompatible:  false,
			expectReason:      "Invalid <= range '<=invalid'",
			expectDetails:     "<= ranges must follow format <=major.minor.patch",
		},
		{
			name:              "invalid < range",
			eventVersion:      "1.0.0",
			subscriptionRange: "<invalid",
			expectCompatible:  false,
			expectReason:      "Invalid < range '<invalid'",
			expectDetails:     "< ranges must follow format <major.minor.patch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := se.CheckSchemaCompatibilityDetailed(tt.eventVersion, tt.subscriptionRange)

			if result.Compatible != tt.expectCompatible {
				t.Errorf("CheckSchemaCompatibilityDetailed() compatible = %v, want %v", result.Compatible, tt.expectCompatible)
			}

			if !contains(result.Reason, tt.expectReason) {
				t.Errorf("CheckSchemaCompatibilityDetailed() reason = %q, want to contain %q", result.Reason, tt.expectReason)
			}

			if !contains(result.Details, tt.expectDetails) {
				t.Errorf("CheckSchemaCompatibilityDetailed() details = %q, want to contain %q", result.Details, tt.expectDetails)
			}
		})
	}
}

// TestEvaluateVersionRangeDetailed tests the detailed version range evaluation function.
func TestEvaluateVersionRangeDetailed(t *testing.T) {
	tests := []struct {
		name              string
		version           SemVer
		rangeSpec         string
		expectCompatible  bool
		expectReasonPart  string
		expectDetailsPart string
	}{
		{
			name:              "exact match success",
			version:           SemVer{1, 2, 3},
			rangeSpec:         "1.2.3",
			expectCompatible:  true,
			expectReasonPart:  "exactly matches",
			expectDetailsPart: "strongest compatibility guarantee",
		},
		{
			name:              "caret range major version difference",
			version:           SemVer{2, 0, 0},
			rangeSpec:         "^1.0.0",
			expectCompatible:  false,
			expectReasonPart:  "different major version",
			expectDetailsPart: "breaking changes",
		},
		{
			name:              "tilde range minor version difference",
			version:           SemVer{1, 3, 0},
			rangeSpec:         "~1.2.0",
			expectCompatible:  false,
			expectReasonPart:  "different major.minor",
			expectDetailsPart: "reject different major or minor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compatible, reason, details := evaluateVersionRangeDetailed(tt.version, tt.rangeSpec)

			if compatible != tt.expectCompatible {
				t.Errorf("evaluateVersionRangeDetailed() compatible = %v, want %v", compatible, tt.expectCompatible)
			}

			if !contains(reason, tt.expectReasonPart) {
				t.Errorf("evaluateVersionRangeDetailed() reason = %q, want to contain %q", reason, tt.expectReasonPart)
			}

			if !contains(details, tt.expectDetailsPart) {
				t.Errorf("evaluateVersionRangeDetailed() details = %q, want to contain %q", details, tt.expectDetailsPart)
			}
		})
	}
}

// TestGetSchemaEvolutionGuidelines tests the schema evolution guidelines function.
func TestGetSchemaEvolutionGuidelines(t *testing.T) {
	guidelines := GetSchemaEvolutionGuidelines()

	expectedKeys := []string{
		"semantic_versioning",
		"version_range_selection",
		"breaking_changes",
		"compatible_changes",
		"bug_fixes",
		"subscription_strategies",
		"migration_planning",
		"best_practices",
		"validation_recommendations",
		"compatibility_matrix",
	}

	// Check that all expected guidelines are present
	for _, key := range expectedKeys {
		if guideline, exists := guidelines[key]; !exists {
			t.Errorf("Expected guideline key %q to exist", key)
		} else if guideline == "" {
			t.Errorf("Expected guideline %q to have non-empty content", key)
		} else if len(guideline) < 50 {
			t.Errorf("Expected guideline %q to have substantial content (got %d chars)", key, len(guideline))
		}
	}

	// Verify specific content expectations
	if semver := guidelines["semantic_versioning"]; !contains(semver, "major.minor.patch") {
		t.Errorf("Expected semantic_versioning guideline to mention 'major.minor.patch', got: %s", semver)
	}

	if breaking := guidelines["breaking_changes"]; !contains(breaking, "MUST increment the major version") {
		t.Errorf("Expected breaking_changes guideline to mention 'MUST increment the major version', got: %s", breaking)
	}

	if matrix := guidelines["compatibility_matrix"]; !contains(matrix, "1.0.0 -> 1.1.0") {
		t.Errorf("Expected compatibility_matrix guideline to include version examples, got: %s", matrix)
	}

	t.Logf("Schema evolution guidelines include %d topics", len(guidelines))
}

// TestSchemaCompatibilityIntegration tests schema compatibility within the context of subscription evaluation.
func TestSchemaCompatibilityIntegration(t *testing.T) {
	se, err := NewSubscriptionEvaluator()
	if err != nil {
		t.Fatalf("Failed to create subscription evaluator: %v", err)
	}

	// Test that schema compatibility is properly integrated into subscription evaluation
	event := Event{
		Type:          "library_built",
		SchemaVersion: "1.2.0",
		Payload: map[string]interface{}{
			"version": "2.1.0",
			"status":  "success",
		},
		Source:    "test-org/library",
		Timestamp: time.Now().Unix(),
	}

	tests := []struct {
		name            string
		schemaVersion   string
		expectMatch     bool
		expectErrorPart string
	}{
		{
			name:          "compatible caret range",
			schemaVersion: "^1.0.0",
			expectMatch:   true,
		},
		{
			name:          "incompatible exact version",
			schemaVersion: "1.1.0",
			expectMatch:   false,
		},
		{
			name:            "invalid schema version range",
			schemaVersion:   "invalid",
			expectMatch:     false,
			expectErrorPart: "Invalid version range",
		},
		{
			name:          "no schema version constraint",
			schemaVersion: "",
			expectMatch:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscription := config.Subscription{
				Events:        []string{"library_built"},
				SchemaVersion: tt.schemaVersion,
				Workflow:      "test_workflow",
			}

			result, err := se.EvaluateSubscription(subscription, event)

			if tt.expectErrorPart != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, but got no error", tt.expectErrorPart)
				} else if !contains(err.Error(), tt.expectErrorPart) {
					t.Errorf("Expected error to contain %q, got: %v", tt.expectErrorPart, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expectMatch {
				t.Errorf("EvaluateSubscription() = %v, want %v", result, tt.expectMatch)
			}
		})
	}
}

// contains checks if a string contains a substring (simple helper for tests).
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	// Simple implementation: check if substr appears in s
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
