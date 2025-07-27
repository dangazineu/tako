package interfaces

import (
	"context"

	"github.com/dangazineu/tako/internal/config"
)

// SubscriptionMatch represents a repository that should be triggered by an event.
type SubscriptionMatch struct {
	RepositoryPath string
	RepositoryName string
	Subscription   config.Subscription
	Config         *config.Config
}

// SubscriptionDiscoverer interface for discovering subscriptions.
type SubscriptionDiscoverer interface {
	DiscoverSubscriptions(ctx context.Context, eventType, artifactRef string, eventPayload map[string]string) ([]SubscriptionMatch, error)
}

// WorkflowRunner interface for executing workflows (allows for testing with mocks).
type WorkflowRunner interface {
	ExecuteChildWorkflow(ctx context.Context, repoPath, workflowName string, inputs map[string]string) (string, error)
}
