package engine

import (
	"fmt"
	"testing"
	"time"

	"github.com/dangazineu/tako/internal/config"
	"github.com/google/cel-go/cel"
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

func TestNewSubscriptionEvaluator_Cache(t *testing.T) {
	se, err := NewSubscriptionEvaluator()
	if err != nil {
		t.Fatalf("Failed to create subscription evaluator: %v", err)
	}

	// Check that cache is initialized
	if se.programCache == nil {
		t.Fatal("Expected non-nil program cache")
	}

	// Check initial cache stats
	hits, misses, size := se.GetCacheStats()
	if hits != 0 || misses != 0 || size != 0 {
		t.Errorf("Expected initial cache stats (0, 0, 0), got (%d, %d, %d)", hits, misses, size)
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

	subscription := config.Subscription{
		Artifact:      "test-org/library:lib",
		Events:        []string{"library_built"},
		SchemaVersion: "^1.0.0",
		Filters:       []string{"event.payload.version >= '2.0.0'"},
		Workflow:      "update-deps",
	}

	// First evaluation - should be a cache miss
	result1, err := se.EvaluateSubscription(subscription, event)
	if err != nil {
		t.Fatalf("First evaluation failed: %v", err)
	}
	if !result1 {
		t.Error("Expected first evaluation to return true")
	}

	// Check cache stats after first evaluation
	hits, misses, size := se.GetCacheStats()
	if hits != 0 || misses != 1 || size != 1 {
		t.Errorf("Expected cache stats after first evaluation (0, 1, 1), got (%d, %d, %d)", hits, misses, size)
	}

	// Second evaluation with same filter - should be a cache hit
	result2, err := se.EvaluateSubscription(subscription, event)
	if err != nil {
		t.Fatalf("Second evaluation failed: %v", err)
	}
	if !result2 {
		t.Error("Expected second evaluation to return true")
	}

	// Check cache stats after second evaluation
	hits, misses, size = se.GetCacheStats()
	if hits != 1 || misses != 1 || size != 1 {
		t.Errorf("Expected cache stats after second evaluation (1, 1, 1), got (%d, %d, %d)", hits, misses, size)
	}

	// Third evaluation with different filter - should be another cache miss
	subscription.Filters = []string{"event.payload.status == 'success'"}
	result3, err := se.EvaluateSubscription(subscription, event)
	if err != nil {
		t.Fatalf("Third evaluation failed: %v", err)
	}
	if !result3 {
		t.Error("Expected third evaluation to return true")
	}

	// Check cache stats after third evaluation
	hits, misses, size = se.GetCacheStats()
	if hits != 1 || misses != 2 || size != 2 {
		t.Errorf("Expected cache stats after third evaluation (1, 2, 2), got (%d, %d, %d)", hits, misses, size)
	}

	// Fourth evaluation with first filter again - should be a cache hit
	subscription.Filters = []string{"event.payload.version >= '2.0.0'"}
	result4, err := se.EvaluateSubscription(subscription, event)
	if err != nil {
		t.Fatalf("Fourth evaluation failed: %v", err)
	}
	if !result4 {
		t.Error("Expected fourth evaluation to return true")
	}

	// Check final cache stats
	hits, misses, size = se.GetCacheStats()
	if hits != 2 || misses != 2 || size != 2 {
		t.Errorf("Expected final cache stats (2, 2, 2), got (%d, %d, %d)", hits, misses, size)
	}
}

func TestSubscriptionEvaluator_CacheThreadSafety(t *testing.T) {
	se, err := NewSubscriptionEvaluator()
	if err != nil {
		t.Fatalf("Failed to create subscription evaluator: %v", err)
	}

	event := Event{
		Type:          "library_built",
		SchemaVersion: "1.0.0",
		Payload: map[string]interface{}{
			"version": "2.1.0",
		},
		Source:    "test-org/library",
		Timestamp: time.Now().Unix(),
	}

	subscription := config.Subscription{
		Artifact:      "test-org/library:lib",
		Events:        []string{"library_built"},
		SchemaVersion: "^1.0.0",
		Filters:       []string{"event.payload.version >= '2.0.0'"},
		Workflow:      "update-deps",
	}

	// Run concurrent evaluations to test thread safety
	const numGoroutines = 10
	const numEvaluations = 100

	done := make(chan bool, numGoroutines)
	errors := make(chan error, numGoroutines*numEvaluations)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()

			for j := 0; j < numEvaluations; j++ {
				result, err := se.EvaluateSubscription(subscription, event)
				if err != nil {
					errors <- err
					return
				}
				if !result {
					errors <- fmt.Errorf("expected evaluation to return true")
					return
				}
			}
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Check for any errors
	close(errors)
	for err := range errors {
		t.Errorf("Concurrent evaluation error: %v", err)
	}

	// Verify cache stats make sense
	hits, misses, size := se.GetCacheStats()
	if misses == 0 {
		t.Error("Expected at least one cache miss")
	}
	if hits == 0 {
		t.Error("Expected at least one cache hit")
	}
	if size == 0 {
		t.Error("Expected cache to contain at least one entry")
	}
}

func TestCELProgramCache_LRUEviction(t *testing.T) {
	// Test LRU eviction with a small cache
	cache := newCELProgramCache(2)

	// Create a simple CEL environment for testing
	env, err := cel.NewEnv(
		cel.Variable("x", cel.IntType),
	)
	if err != nil {
		t.Fatalf("Failed to create CEL environment: %v", err)
	}

	// Create three different programs
	expressions := []string{"x > 0", "x < 10", "x == 5"}
	programs := make([]cel.Program, len(expressions))

	for i, expr := range expressions {
		ast, issues := env.Compile(expr)
		if issues != nil && issues.Err() != nil {
			t.Fatalf("Failed to compile expression %s: %v", expr, issues.Err())
		}

		program, err := env.Program(ast)
		if err != nil {
			t.Fatalf("Failed to create program for %s: %v", expr, err)
		}
		programs[i] = program
	}

	// Add first two programs
	cache.put("expr1", programs[0])
	cache.put("expr2", programs[1])

	_, _, size := cache.stats()
	if size != 2 {
		t.Errorf("Expected cache size 2, got %d", size)
	}

	// Verify both are retrievable
	if _, found := cache.get("expr1"); !found {
		t.Error("Expected to find expr1 in cache")
	}
	if _, found := cache.get("expr2"); !found {
		t.Error("Expected to find expr2 in cache")
	}

	// Add third program - should evict the least recently used (expr1)
	cache.put("expr3", programs[2])

	_, _, size = cache.stats()
	if size != 2 {
		t.Errorf("Expected cache size 2 after eviction, got %d", size)
	}

	// expr1 should be evicted, expr2 and expr3 should remain
	if _, found := cache.get("expr1"); found {
		t.Error("Expected expr1 to be evicted from cache")
	}
	if _, found := cache.get("expr2"); !found {
		t.Error("Expected expr2 to remain in cache")
	}
	if _, found := cache.get("expr3"); !found {
		t.Error("Expected expr3 to be in cache")
	}
}

func TestSubscriptionEvaluator_CompoundVersionRanges(t *testing.T) {
	se, err := NewSubscriptionEvaluator()
	if err != nil {
		t.Fatalf("Failed to create subscription evaluator: %v", err)
	}

	tests := []struct {
		name              string
		eventVersion      string
		subscriptionRange string
		compatible        bool
		expectError       bool
	}{
		// Compound ranges - basic
		{
			name:              "compound range >=1.0.0 <2.0.0 - compatible",
			eventVersion:      "1.5.0",
			subscriptionRange: ">=1.0.0 <2.0.0",
			compatible:        true,
		},
		{
			name:              "compound range >=1.0.0 <2.0.0 - not compatible (too low)",
			eventVersion:      "0.9.0",
			subscriptionRange: ">=1.0.0 <2.0.0",
			compatible:        false,
		},
		{
			name:              "compound range >=1.0.0 <2.0.0 - not compatible (too high)",
			eventVersion:      "2.0.0",
			subscriptionRange: ">=1.0.0 <2.0.0",
			compatible:        false,
		},
		{
			name:              "compound range >=1.0.0 <=2.0.0 - compatible (equal upper bound)",
			eventVersion:      "2.0.0",
			subscriptionRange: ">=1.0.0 <=2.0.0",
			compatible:        true,
		},
		{
			name:              "compound range >1.0.0 <2.0.0 - compatible",
			eventVersion:      "1.5.0",
			subscriptionRange: ">1.0.0 <2.0.0",
			compatible:        true,
		},
		{
			name:              "compound range >1.0.0 <2.0.0 - not compatible (equal lower bound)",
			eventVersion:      "1.0.0",
			subscriptionRange: ">1.0.0 <2.0.0",
			compatible:        false,
		},
		// Triple compound ranges
		{
			name:              "triple compound range >=1.0.0 <2.0.0 >=1.2.0 - compatible",
			eventVersion:      "1.5.0",
			subscriptionRange: ">=1.0.0 <2.0.0 >=1.2.0",
			compatible:        true,
		},
		{
			name:              "triple compound range >=1.0.0 <2.0.0 >=1.2.0 - not compatible",
			eventVersion:      "1.1.0",
			subscriptionRange: ">=1.0.0 <2.0.0 >=1.2.0",
			compatible:        false,
		},
		// Mixed with caret and tilde
		{
			name:              "compound with caret ^1.0.0 <1.5.0 - compatible",
			eventVersion:      "1.3.0",
			subscriptionRange: "^1.0.0 <1.5.0",
			compatible:        true,
		},
		{
			name:              "compound with caret ^1.0.0 <1.5.0 - not compatible (version too high)",
			eventVersion:      "1.6.0",
			subscriptionRange: "^1.0.0 <1.5.0",
			compatible:        false,
		},
		{
			name:              "compound with tilde ~1.2.0 >=1.2.5 - compatible",
			eventVersion:      "1.2.8",
			subscriptionRange: "~1.2.0 >=1.2.5",
			compatible:        true,
		},
		{
			name:              "compound with tilde ~1.2.0 >=1.2.5 - not compatible",
			eventVersion:      "1.2.3",
			subscriptionRange: "~1.2.0 >=1.2.5",
			compatible:        false,
		},
		// Error cases
		{
			name:              "compound range with invalid first component",
			eventVersion:      "1.0.0",
			subscriptionRange: ">=invalid <2.0.0",
			compatible:        false,
			expectError:       true,
		},
		{
			name:              "compound range with invalid second component",
			eventVersion:      "1.0.0",
			subscriptionRange: ">=1.0.0 <invalid",
			compatible:        false,
			expectError:       true,
		},
		// Edge cases
		{
			name:              "compound range with extra spaces",
			eventVersion:      "1.5.0",
			subscriptionRange: "  >=1.0.0   <2.0.0  ",
			compatible:        true,
		},
		{
			name:              "compound range with single component (should work)",
			eventVersion:      "1.5.0",
			subscriptionRange: ">=1.0.0",
			compatible:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compatible, err := se.CheckSchemaCompatibility(tt.eventVersion, tt.subscriptionRange)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for range '%s', but got none", tt.subscriptionRange)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for range '%s': %v", tt.subscriptionRange, err)
				return
			}

			if compatible != tt.compatible {
				t.Errorf("CheckSchemaCompatibility(%s, %s) = %v, want %v",
					tt.eventVersion, tt.subscriptionRange, compatible, tt.compatible)
			}
		})
	}
}

func TestEvaluateCompoundVersionRange_Directly(t *testing.T) {
	tests := []struct {
		name        string
		version     SemVer
		rangeSpec   string
		expected    bool
		expectError bool
	}{
		{
			name:      "basic compound range - in range",
			version:   SemVer{Major: 1, Minor: 5, Patch: 0},
			rangeSpec: ">=1.0.0 <2.0.0",
			expected:  true,
		},
		{
			name:      "basic compound range - below range",
			version:   SemVer{Major: 0, Minor: 9, Patch: 0},
			rangeSpec: ">=1.0.0 <2.0.0",
			expected:  false,
		},
		{
			name:      "basic compound range - above range",
			version:   SemVer{Major: 2, Minor: 0, Patch: 0},
			rangeSpec: ">=1.0.0 <2.0.0",
			expected:  false,
		},
		{
			name:      "narrow compound range",
			version:   SemVer{Major: 1, Minor: 2, Patch: 5},
			rangeSpec: ">=1.2.0 <=1.2.10",
			expected:  true,
		},
		{
			name:      "contradictory compound range",
			version:   SemVer{Major: 1, Minor: 5, Patch: 0},
			rangeSpec: ">=2.0.0 <1.0.0",
			expected:  false,
		},
		{
			name:        "invalid range component",
			version:     SemVer{Major: 1, Minor: 0, Patch: 0},
			rangeSpec:   ">=1.0.0 <invalid",
			expected:    false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateCompoundVersionRange(tt.version, tt.rangeSpec)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("evaluateCompoundVersionRange(%+v, %s) = %v, want %v",
					tt.version, tt.rangeSpec, result, tt.expected)
			}
		})
	}
}
