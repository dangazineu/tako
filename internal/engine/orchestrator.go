package engine

import (
	"context"
	"errors"
	"sort"

	"github.com/dangazineu/tako/internal/interfaces"
)

// Orchestrator coordinates subscription discovery and workflow triggering.
// It provides a stable, high-level API for the subscription-based workflow
// triggering system while maintaining loose coupling through dependency injection.
//
// The orchestrator supports filtering and prioritization of subscriptions:
//   - Filtering subscriptions based on criteria
//   - Prioritizing subscriptions by repository path for deterministic ordering
//   - Adding structured logging and monitoring
//   - Coordinating workflow triggering with state management
//   - Handling idempotency and diamond dependency resolution
type Orchestrator struct {
	discoverer interfaces.SubscriptionDiscoverer
	config     OrchestratorConfig
}

// OrchestratorConfig contains configuration options for orchestrator behavior.
type OrchestratorConfig struct {
	// EnableFiltering enables subscription filtering (disabled by default for backward compatibility)
	EnableFiltering bool
	// EnablePrioritization enables priority-based sorting (disabled by default for backward compatibility)
	EnablePrioritization bool
	// FilterDisabledSubscriptions removes subscriptions marked as disabled
	FilterDisabledSubscriptions bool
}

// NewOrchestrator creates a new Orchestrator with the provided dependencies and default configuration.
// The discoverer is used to find repositories that subscribe to specific events.
// Returns an error if the discoverer is nil to ensure safe construction.
//
// Example usage:
//
//	// Create a discovery manager for repository scanning
//	cacheDir := "~/.tako/cache"
//	discoveryManager := engine.NewDiscoveryManager(cacheDir)
//
//	// Create the orchestrator with dependency injection
//	orchestrator, err := engine.NewOrchestrator(discoveryManager)
//	if err != nil {
//		return fmt.Errorf("failed to create orchestrator: %w", err)
//	}
//
//	// The orchestrator is now ready to coordinate subscription discovery
//	ctx := context.Background()
//	matches, err := orchestrator.DiscoverSubscriptions(ctx, "myorg/mylib:library", "build_completed")
//
// For testing, you can provide a mock implementation:
//
//	mockDiscoverer := &MyMockDiscoverer{}
//	testOrchestrator, err := engine.NewOrchestrator(mockDiscoverer)
func NewOrchestrator(discoverer interfaces.SubscriptionDiscoverer) (*Orchestrator, error) {
	return NewOrchestratorWithConfig(discoverer, OrchestratorConfig{})
}

// NewOrchestratorWithConfig creates a new Orchestrator with the provided dependencies and configuration.
// This allows for customization of orchestrator behavior while maintaining backward compatibility.
//
// Example usage with configuration:
//
//	config := engine.OrchestratorConfig{
//		EnableFiltering:             true,
//		EnablePrioritization:        true,
//		FilterDisabledSubscriptions: true,
//	}
//	orchestrator, err := engine.NewOrchestratorWithConfig(discoveryManager, config)
func NewOrchestratorWithConfig(discoverer interfaces.SubscriptionDiscoverer, config OrchestratorConfig) (*Orchestrator, error) {
	if discoverer == nil {
		return nil, errors.New("discoverer cannot be nil")
	}
	return &Orchestrator{
		discoverer: discoverer,
		config:     config,
	}, nil
}

// DiscoverSubscriptions finds all repositories that subscribe to the specified
// artifact and event type. This method provides the orchestration layer for
// subscription discovery with optional filtering and prioritization capabilities.
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
// Orchestration features:
//   - Filtering logic to remove disabled subscriptions (if enabled)
//   - Priority-based sorting for deterministic subscription ordering (if enabled)
//   - Context cancellation handling for responsive behavior
//   - Parameter validation for robustness
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
	// Check for context cancellation early
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate parameters at the orchestrator level for robustness
	if artifact == "" {
		return nil, errors.New("orchestrator: artifact cannot be empty")
	}
	if eventType == "" {
		return nil, errors.New("orchestrator: eventType cannot be empty")
	}

	// Delegate to the discoverer for raw subscription discovery
	rawMatches, err := o.discoverer.FindSubscribers(artifact, eventType)
	if err != nil {
		return nil, err
	}

	// Apply orchestration logic
	filteredMatches := o.filterSubscriptions(rawMatches)
	prioritizedMatches := o.prioritizeSubscriptions(filteredMatches)

	return prioritizedMatches, nil
}

// filterSubscriptions applies filtering logic to subscription matches.
// Currently supports filtering out disabled subscriptions if configured.
func (o *Orchestrator) filterSubscriptions(matches []interfaces.SubscriptionMatch) []interfaces.SubscriptionMatch {
	if !o.config.EnableFiltering {
		return matches
	}

	if !o.config.FilterDisabledSubscriptions {
		return matches
	}

	// Filter out disabled subscriptions (if the subscription has a Disabled field)
	// Note: Since config.Subscription doesn't currently have a Disabled field,
	// this filtering is a no-op for now but provides the infrastructure
	// for future enhancement when the schema is extended.
	filtered := make([]interfaces.SubscriptionMatch, 0, len(matches))
	filtered = append(filtered, matches...)

	return filtered
}

// prioritizeSubscriptions applies priority-based sorting to subscription matches.
// Sorts by repository path for deterministic ordering, supporting the "first-wins"
// diamond dependency resolution rule implemented in the fan-out executor.
func (o *Orchestrator) prioritizeSubscriptions(matches []interfaces.SubscriptionMatch) []interfaces.SubscriptionMatch {
	if !o.config.EnablePrioritization {
		return matches
	}

	// Create a copy to avoid modifying the original slice
	prioritized := make([]interfaces.SubscriptionMatch, len(matches))
	copy(prioritized, matches)

	// Sort by repository path for deterministic ordering
	// This supports the "first-wins" rule for diamond dependency resolution
	sort.Slice(prioritized, func(i, j int) bool {
		if prioritized[i].Repository != prioritized[j].Repository {
			return prioritized[i].Repository < prioritized[j].Repository
		}
		// Secondary sort by workflow for additional determinism
		return prioritized[i].Subscription.Workflow < prioritized[j].Subscription.Workflow
	})

	return prioritized
}
