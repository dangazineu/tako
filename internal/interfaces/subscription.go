// Package interfaces defines the core interfaces for the Tako subscription-based triggering system.
//
// This package provides interface definitions that enable dependency injection and testability
// by decoupling concrete implementations from their consumers. The main interfaces are:
//
//   - SubscriptionDiscoverer: For discovering repositories that subscribe to events
//   - WorkflowRunner: For executing workflows in repositories
//
// These interfaces are implemented by types in the engine package and consumed by step executors
// in the steps package, creating a clean separation of concerns and enabling easier testing.
package interfaces

// SubscriptionDiscoverer defines the interface for discovering repositories that subscribe to events.
// Implementations of this interface are responsible for finding all repositories that have
// subscribed to a specific artifact and event type combination.
type SubscriptionDiscoverer interface {
	// FindSubscribers finds all repositories that subscribe to the specified artifact and event type.
	// Returns a sorted list of subscription matches for deterministic behavior.
	//
	// Parameters:
	//   - artifact: The artifact identifier in "owner/repo:artifact" format
	//   - eventType: The type of event (e.g., "build_completed", "test_passed")
	//
	// Returns:
	//   - []SubscriptionMatch: List of matching subscriptions with repository information
	//   - error: An error if the discovery process fails
	FindSubscribers(artifact, eventType string) ([]SubscriptionMatch, error)
}
