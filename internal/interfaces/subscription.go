// Package interfaces defines the core interfaces for the Tako subscription-based triggering system.
// These interfaces enable dependency injection and testability by decoupling implementations.
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
