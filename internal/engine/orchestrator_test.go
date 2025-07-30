package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/interfaces"
)

// mockSubscriptionDiscoverer implements the SubscriptionDiscoverer interface for testing.
// It allows tests to control the behavior of the discoverer without requiring
// real file system operations or actual repository scanning.
type mockSubscriptionDiscoverer struct {
	findSubscribersFunc func(artifact, eventType string) ([]interfaces.SubscriptionMatch, error)
}

// FindSubscribers implements the SubscriptionDiscoverer interface.
func (m *mockSubscriptionDiscoverer) FindSubscribers(artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
	if m.findSubscribersFunc != nil {
		return m.findSubscribersFunc(artifact, eventType)
	}
	return []interfaces.SubscriptionMatch{}, nil
}

func TestNewOrchestrator(t *testing.T) {
	t.Run("valid discoverer", func(t *testing.T) {
		discoverer := &mockSubscriptionDiscoverer{}

		orchestrator, err := NewOrchestrator(discoverer)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if orchestrator == nil {
			t.Fatal("Expected non-nil orchestrator")
		}

		if orchestrator.discoverer != discoverer {
			t.Error("Expected discoverer to be set correctly")
		}
	})

	t.Run("nil discoverer should return error", func(t *testing.T) {
		orchestrator, err := NewOrchestrator(nil)

		if err == nil {
			t.Fatal("Expected error for nil discoverer, got nil")
		}

		if orchestrator != nil {
			t.Error("Expected nil orchestrator when error occurs")
		}

		expectedErrorMsg := "discoverer cannot be nil"
		if err.Error() != expectedErrorMsg {
			t.Errorf("Expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
		}
	})
}

func TestOrchestrator_DiscoverSubscriptions_HappyPath(t *testing.T) {
	// Prepare test data
	expectedMatches := []interfaces.SubscriptionMatch{
		{
			Repository: "test-org/consumer1",
			Subscription: config.Subscription{
				Artifact: "test-org/library:lib",
				Events:   []string{"build_completed"},
				Workflow: "update",
			},
			RepoPath: "/path/to/consumer1",
		},
		{
			Repository: "test-org/consumer2",
			Subscription: config.Subscription{
				Artifact: "test-org/library:lib",
				Events:   []string{"build_completed"},
				Workflow: "deploy",
			},
			RepoPath: "/path/to/consumer2",
		},
	}

	// Create mock discoverer that returns our test data
	discoverer := &mockSubscriptionDiscoverer{
		findSubscribersFunc: func(artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
			if artifact == "test-org/library:lib" && eventType == "build_completed" {
				return expectedMatches, nil
			}
			return []interfaces.SubscriptionMatch{}, nil
		},
	}

	orchestrator, err := NewOrchestrator(discoverer)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	ctx := context.Background()

	// Execute the method under test
	matches, err := orchestrator.DiscoverSubscriptions(ctx, "test-org/library:lib", "build_completed")

	// Verify results
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("Expected 2 matches, got %d", len(matches))
	}

	// Verify first match
	if matches[0].Repository != "test-org/consumer1" {
		t.Errorf("Expected first match repository to be 'test-org/consumer1', got '%s'", matches[0].Repository)
	}
	if matches[0].Subscription.Workflow != "update" {
		t.Errorf("Expected first match workflow to be 'update', got '%s'", matches[0].Subscription.Workflow)
	}

	// Verify second match
	if matches[1].Repository != "test-org/consumer2" {
		t.Errorf("Expected second match repository to be 'test-org/consumer2', got '%s'", matches[1].Repository)
	}
	if matches[1].Subscription.Workflow != "deploy" {
		t.Errorf("Expected second match workflow to be 'deploy', got '%s'", matches[1].Subscription.Workflow)
	}
}

func TestOrchestrator_DiscoverSubscriptions_ErrorPath(t *testing.T) {
	expectedError := errors.New("discoverer failed")

	// Create mock discoverer that returns an error
	discoverer := &mockSubscriptionDiscoverer{
		findSubscribersFunc: func(artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
			return nil, expectedError
		},
	}

	orchestrator, err := NewOrchestrator(discoverer)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	ctx := context.Background()

	// Execute the method under test
	matches, err := orchestrator.DiscoverSubscriptions(ctx, "test-org/library:lib", "build_completed")

	// Verify error handling
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}

	if err != expectedError {
		t.Errorf("Expected error to be '%v', got '%v'", expectedError, err)
	}

	if matches != nil {
		t.Errorf("Expected matches to be nil on error, got %v", matches)
	}
}

func TestOrchestrator_DiscoverSubscriptions_NoMatches(t *testing.T) {
	// Create mock discoverer that returns empty results
	discoverer := &mockSubscriptionDiscoverer{
		findSubscribersFunc: func(artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
			return []interfaces.SubscriptionMatch{}, nil
		},
	}

	orchestrator, err := NewOrchestrator(discoverer)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	ctx := context.Background()

	// Execute the method under test
	matches, err := orchestrator.DiscoverSubscriptions(ctx, "nonexistent/repo:artifact", "unknown_event")

	// Verify results
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(matches) != 0 {
		t.Errorf("Expected 0 matches, got %d", len(matches))
	}
}

func TestOrchestrator_DiscoverSubscriptions_ParameterValidation(t *testing.T) {
	tests := []struct {
		name      string
		artifact  string
		eventType string
		expectErr bool
		errMsg    string
	}{
		{
			name:      "empty artifact",
			artifact:  "",
			eventType: "build_completed",
			expectErr: true,
			errMsg:    "orchestrator: artifact cannot be empty",
		},
		{
			name:      "empty event type",
			artifact:  "test-org/repo:lib",
			eventType: "",
			expectErr: true,
			errMsg:    "orchestrator: eventType cannot be empty",
		},
		{
			name:      "both empty",
			artifact:  "",
			eventType: "",
			expectErr: true,
			errMsg:    "orchestrator: artifact cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock discoverer - parameter validation now happens at orchestrator level
			discoverer := &mockSubscriptionDiscoverer{
				findSubscribersFunc: func(artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
					return []interfaces.SubscriptionMatch{}, nil
				},
			}

			orchestrator, err := NewOrchestrator(discoverer)
			if err != nil {
				t.Fatalf("Failed to create orchestrator: %v", err)
			}
			ctx := context.Background()

			// Execute the method under test
			matches, err := orchestrator.DiscoverSubscriptions(ctx, tt.artifact, tt.eventType)

			// Verify error handling
			if tt.expectErr {
				if err == nil {
					t.Fatal("Expected an error, got nil")
				}
				if err.Error() != tt.errMsg {
					t.Errorf("Expected error message '%s', got '%s'", tt.errMsg, err.Error())
				}
				if matches != nil {
					t.Errorf("Expected matches to be nil on error, got %v", matches)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestOrchestrator_DiscoverSubscriptions_ContextHandling(t *testing.T) {
	// Create mock discoverer that simulates context cancellation checking
	discoverer := &mockSubscriptionDiscoverer{
		findSubscribersFunc: func(artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
			// In a real implementation, the discoverer would check ctx.Done()
			// For this test, we simulate successful execution
			return []interfaces.SubscriptionMatch{
				{
					Repository: "test-org/repo",
					Subscription: config.Subscription{
						Artifact: artifact,
						Events:   []string{eventType},
						Workflow: "test_workflow",
					},
					RepoPath: "/path/to/repo",
				},
			}, nil
		},
	}

	orchestrator, err := NewOrchestrator(discoverer)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}

	t.Run("valid context", func(t *testing.T) {
		ctx := context.Background()
		matches, err := orchestrator.DiscoverSubscriptions(ctx, "test-org/lib:lib", "build_completed")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(matches) != 1 {
			t.Errorf("Expected 1 match, got %d", len(matches))
		}
	})

	t.Run("context with timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		matches, err := orchestrator.DiscoverSubscriptions(ctx, "test-org/lib:lib", "build_completed")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(matches) != 1 {
			t.Errorf("Expected 1 match, got %d", len(matches))
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// The orchestrator should now check for context cancellation
		matches, err := orchestrator.DiscoverSubscriptions(ctx, "test-org/lib:lib", "build_completed")

		// Should return context cancellation error
		if err == nil {
			t.Fatal("Expected context cancellation error, got nil")
		}
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got: %v", err)
		}
		if matches != nil {
			t.Errorf("Expected nil matches on context cancellation, got %v", matches)
		}
	})
}

func TestOrchestrator_DiscoverSubscriptions_EdgeCases(t *testing.T) {
	t.Run("nil discoverer handled during construction", func(t *testing.T) {
		// With the new constructor, nil discoverers are rejected at construction time
		orchestrator, err := NewOrchestrator(nil)

		if err == nil {
			t.Fatal("Expected error when creating orchestrator with nil discoverer")
		}

		if orchestrator != nil {
			t.Error("Expected nil orchestrator when error occurs")
		}

		expectedErrorMsg := "discoverer cannot be nil"
		if err.Error() != expectedErrorMsg {
			t.Errorf("Expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
		}
	})

	t.Run("large result set", func(t *testing.T) {
		// Test with a large number of subscription matches
		expectedMatches := make([]interfaces.SubscriptionMatch, 100)
		for i := 0; i < 100; i++ {
			expectedMatches[i] = interfaces.SubscriptionMatch{
				Repository: fmt.Sprintf("test-org/repo%d", i),
				Subscription: config.Subscription{
					Artifact: "test-org/lib:lib",
					Events:   []string{"build_completed"},
					Workflow: fmt.Sprintf("workflow%d", i),
				},
				RepoPath: fmt.Sprintf("/path/to/repo%d", i),
			}
		}

		discoverer := &mockSubscriptionDiscoverer{
			findSubscribersFunc: func(artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
				return expectedMatches, nil
			},
		}

		orchestrator, err := NewOrchestrator(discoverer)
		if err != nil {
			t.Fatalf("Failed to create orchestrator: %v", err)
		}
		ctx := context.Background()

		matches, err := orchestrator.DiscoverSubscriptions(ctx, "test-org/lib:lib", "build_completed")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(matches) != 100 {
			t.Errorf("Expected 100 matches, got %d", len(matches))
		}
		// Verify first and last matches to ensure data integrity
		if matches[0].Repository != "test-org/repo0" {
			t.Errorf("Expected first match repository to be 'test-org/repo0', got '%s'", matches[0].Repository)
		}
		if matches[99].Repository != "test-org/repo99" {
			t.Errorf("Expected last match repository to be 'test-org/repo99', got '%s'", matches[99].Repository)
		}
	})
}

// TestOrchestrator_Integration_WithDiscoveryManager tests that the orchestrator
// works correctly with the real DiscoveryManager component, demonstrating
// end-to-end integration without requiring external dependencies.
func TestOrchestrator_Integration_WithDiscoveryManager(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a real DiscoveryManager
	discoveryManager := NewDiscoveryManager(tempDir)

	// Create the orchestrator with the real discoverer
	orchestrator, err := NewOrchestrator(discoveryManager)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}

	ctx := context.Background()

	t.Run("integration with empty cache", func(t *testing.T) {
		// Test with empty cache directory (no repositories)
		matches, err := orchestrator.DiscoverSubscriptions(ctx, "test-org/lib:library", "build_completed")

		if err != nil {
			t.Fatalf("Expected no error with empty cache, got: %v", err)
		}

		// Should return empty results, not an error
		if len(matches) != 0 {
			t.Errorf("Expected 0 matches with empty cache, got %d", len(matches))
		}
	})

	t.Run("integration validates parameters at orchestrator level", func(t *testing.T) {
		// Test that parameter validation is handled by the orchestrator
		matches, err := orchestrator.DiscoverSubscriptions(ctx, "", "build_completed")

		// The orchestrator should return an error for empty artifact
		if err == nil {
			t.Fatal("Expected error for empty artifact, got nil")
		}

		if err.Error() != "orchestrator: artifact cannot be empty" {
			t.Errorf("Expected 'orchestrator: artifact cannot be empty' error, got: %v", err)
		}

		if matches != nil {
			t.Errorf("Expected nil matches on error, got: %v", matches)
		}
	})

	t.Run("integration test documents real usage pattern", func(t *testing.T) {
		// This test documents how the orchestrator should be used in practice
		// It demonstrates the integration without requiring external resources

		// 1. Create components
		cacheDir := tempDir
		realDiscoverer := NewDiscoveryManager(cacheDir)
		realOrchestrator, err := NewOrchestrator(realDiscoverer)
		if err != nil {
			t.Fatalf("Failed to create orchestrator: %v", err)
		}

		// 2. Use the orchestrator
		ctx := context.Background()
		matches, err := realOrchestrator.DiscoverSubscriptions(ctx, "example-org/example-lib:library", "build_completed")

		// 3. Verify behavior (no repositories cached, so empty results)
		if err != nil {
			t.Fatalf("Integration test failed: %v", err)
		}

		if len(matches) != 0 {
			t.Errorf("Integration test expected 0 matches, got %d", len(matches))
		}

		// This confirms the integration works and the orchestrator successfully
		// delegates to the real discoverer component
		t.Log("Integration test passed: Orchestrator successfully integrated with DiscoveryManager")
	})

	t.Run("integration demonstrates orchestrator abstraction", func(t *testing.T) {
		// This test shows that the orchestrator provides a stable interface
		// regardless of the underlying discoverer implementation

		// Test with multiple different discoverers through the same orchestrator interface
		discoverers := []interfaces.SubscriptionDiscoverer{
			NewDiscoveryManager(tempDir),  // Real implementation
			&mockSubscriptionDiscoverer{}, // Mock implementation
		}

		for i, discoverer := range discoverers {
			orchestrator, err := NewOrchestrator(discoverer)
			if err != nil {
				t.Fatalf("Failed to create orchestrator: %v", err)
			}
			matches, err := orchestrator.DiscoverSubscriptions(ctx, "test-org/test-lib:lib", "test_event")

			// Both should work without error (though results may differ)
			if err != nil {
				t.Errorf("Discoverer %d failed: %v", i, err)
			}

			// Results should be a valid slice (not nil), even if empty
			// Note: DiscoveryManager returns empty slice, mock returns empty slice by default
			if matches == nil {
				t.Errorf("Discoverer %d returned nil matches (should be empty slice)", i)
			}

			t.Logf("Discoverer %d returned %d matches", i, len(matches))
		}

		t.Log("Integration test passed: Orchestrator abstraction works with multiple discoverer implementations")
	})
}
