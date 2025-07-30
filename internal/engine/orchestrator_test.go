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
	discoverer := &mockSubscriptionDiscoverer{}

	orchestrator := NewOrchestrator(discoverer)

	if orchestrator == nil {
		t.Fatal("Expected non-nil orchestrator")
	}

	if orchestrator.discoverer != discoverer {
		t.Error("Expected discoverer to be set correctly")
	}
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

	orchestrator := NewOrchestrator(discoverer)
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

	orchestrator := NewOrchestrator(discoverer)
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

	orchestrator := NewOrchestrator(discoverer)
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
			errMsg:    "artifact cannot be empty",
		},
		{
			name:      "empty event type",
			artifact:  "test-org/repo:lib",
			eventType: "",
			expectErr: true,
			errMsg:    "event type cannot be empty",
		},
		{
			name:      "both empty",
			artifact:  "",
			eventType: "",
			expectErr: true,
			errMsg:    "artifact cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock discoverer that validates parameters
			discoverer := &mockSubscriptionDiscoverer{
				findSubscribersFunc: func(artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
					if artifact == "" {
						return nil, errors.New("artifact cannot be empty")
					}
					if eventType == "" {
						return nil, errors.New("event type cannot be empty")
					}
					return []interfaces.SubscriptionMatch{}, nil
				},
			}

			orchestrator := NewOrchestrator(discoverer)
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

	orchestrator := NewOrchestrator(discoverer)

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

		// Note: The current implementation doesn't check context cancellation
		// This test documents the current behavior and can be updated when
		// context handling is added to the orchestrator
		matches, err := orchestrator.DiscoverSubscriptions(ctx, "test-org/lib:lib", "build_completed")

		// Current behavior: ignores cancelled context (passes through to discoverer)
		if err != nil {
			t.Fatalf("Expected no error with current implementation, got: %v", err)
		}
		if len(matches) != 1 {
			t.Errorf("Expected 1 match with current implementation, got %d", len(matches))
		}
	})
}

func TestOrchestrator_DiscoverSubscriptions_EdgeCases(t *testing.T) {
	t.Run("nil discoverer should panic on construction", func(t *testing.T) {
		// This test ensures we document the behavior with nil dependencies
		// In Go, calling methods on nil pointers panics, which is expected behavior
		defer func() {
			if r := recover(); r != nil {
				// Expected behavior - method call on nil pointer panics
				t.Log("Correctly panicked when calling method on orchestrator with nil discoverer")
			}
		}()

		orchestrator := NewOrchestrator(nil)
		ctx := context.Background()

		// This should panic
		_, _ = orchestrator.DiscoverSubscriptions(ctx, "test/repo:lib", "event")
		t.Error("Expected panic when calling method on orchestrator with nil discoverer")
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

		orchestrator := NewOrchestrator(discoverer)
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
