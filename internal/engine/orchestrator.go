// Package engine implements the core orchestration and execution logic for Tako.
//
// The orchestrator component provides a high-level coordination layer for
// subscription-based workflow triggering. It acts as an abstraction layer
// over lower-level discovery components and serves as the central hub for
// future orchestration features like filtering, prioritization, and logging.
package engine

import (
	"context"

	"github.com/dangazineu/tako/internal/interfaces"
)

// Orchestrator coordinates subscription discovery and workflow triggering.
// It provides a stable, high-level API for the subscription-based workflow
// triggering system while maintaining loose coupling through dependency injection.
//
// The orchestrator is designed to be extensible for future enhancements:
//   - Filtering subscriptions based on criteria
//   - Prioritizing subscriptions
//   - Adding structured logging and monitoring
//   - Coordinating workflow triggering with state management
//   - Handling idempotency and diamond dependency resolution
type Orchestrator struct {
	discoverer interfaces.SubscriptionDiscoverer
}

// NewOrchestrator creates a new Orchestrator with the provided dependencies.
// The discoverer is used to find repositories that subscribe to specific events.
//
// Example usage:
//
//	discoveryManager := engine.NewDiscoveryManager(cacheDir)
//	orchestrator := engine.NewOrchestrator(discoveryManager)
func NewOrchestrator(discoverer interfaces.SubscriptionDiscoverer) *Orchestrator {
	return &Orchestrator{
		discoverer: discoverer,
	}
}

// DiscoverSubscriptions finds all repositories that subscribe to the specified
// artifact and event type. This method provides the orchestration layer for
// subscription discovery, currently implementing a pass-through to the underlying
// discoverer with plans for future enhancement.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - artifact: The artifact identifier in "owner/repo:artifact" format
//   - eventType: The type of event (e.g., "build_completed", "test_passed")
//
// Returns:
//   - []interfaces.SubscriptionMatch: List of matching subscriptions with repository information
//   - error: An error if the discovery process fails
//
// Future enhancements will add:
//   - Filtering logic to remove disabled subscriptions
//   - Priority-based sorting for subscription ordering
//   - Structured logging for discovery operations
//   - Metrics collection for monitoring
//
// Example usage:
//
//	matches, err := orchestrator.DiscoverSubscriptions(ctx, "myorg/mylib:library", "build_completed")
//	if err != nil {
//	    return fmt.Errorf("failed to discover subscriptions: %w", err)
//	}
//	for _, match := range matches {
//	    fmt.Printf("Found subscription in %s for workflow %s\n", match.Repository, match.Subscription.Workflow)
//	}
func (o *Orchestrator) DiscoverSubscriptions(ctx context.Context, artifact, eventType string) ([]interfaces.SubscriptionMatch, error) {
	// Current implementation: simple pass-through to the discoverer
	// Future orchestration logic will be added here
	return o.discoverer.FindSubscribers(artifact, eventType)
}
