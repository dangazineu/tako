package engine

import (
	"context"
	"errors"
	"testing"

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
