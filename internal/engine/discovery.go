package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/interfaces"
)

// SubscriptionMatch is now defined in the interfaces package.
type SubscriptionMatch = interfaces.SubscriptionMatch

// DiscoveryManager handles repository discovery and subscription lookup.
type DiscoveryManager struct {
	cacheDir string
}

// NewDiscoveryManager creates a new discovery manager with the specified cache directory.
func NewDiscoveryManager(cacheDir string) *DiscoveryManager {
	return &DiscoveryManager{
		cacheDir: cacheDir,
	}
}

// _ ensures DiscoveryManager implements the SubscriptionDiscoverer interface.
// This compile-time check verifies interface compliance.
var _ interfaces.SubscriptionDiscoverer = (*DiscoveryManager)(nil)

// FindSubscribers finds all repositories that subscribe to the specified artifact and event type.
// Returns a sorted list of subscription matches for deterministic behavior.
func (dm *DiscoveryManager) FindSubscribers(artifact, eventType string) ([]SubscriptionMatch, error) {
	if artifact == "" {
		return nil, fmt.Errorf("artifact cannot be empty")
	}
	if eventType == "" {
		return nil, fmt.Errorf("event type cannot be empty")
	}

	matches := make([]SubscriptionMatch, 0)

	// Scan the cache directory for repositories
	repoBaseDir := filepath.Join(dm.cacheDir, "repos")
	if _, err := os.Stat(repoBaseDir); os.IsNotExist(err) {
		// No cached repositories - this is not an error, just return empty results
		return matches, nil
	}

	// Walk through owner directories (first level)
	ownerEntries, err := os.ReadDir(repoBaseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache directory: %v", err)
	}

	for _, ownerEntry := range ownerEntries {
		if !ownerEntry.IsDir() {
			continue
		}

		ownerPath := filepath.Join(repoBaseDir, ownerEntry.Name())

		// Walk through repo directories (second level)
		repoEntries, err := os.ReadDir(ownerPath)
		if err != nil {
			continue // Skip directories we can't read
		}

		for _, repoEntry := range repoEntries {
			if !repoEntry.IsDir() {
				continue
			}

			repoPath := filepath.Join(ownerPath, repoEntry.Name())
			repoName := fmt.Sprintf("%s/%s", ownerEntry.Name(), repoEntry.Name())

			// Check for main branch directory (default branch)
			mainBranchPath := filepath.Join(repoPath, "main")
			if _, err := os.Stat(mainBranchPath); os.IsNotExist(err) {
				continue // Skip if main branch doesn't exist
			}

			// Load subscriptions from this repository
			subscriptions, err := dm.LoadSubscriptions(mainBranchPath)
			if err != nil {
				continue // Skip repositories with loading errors
			}

			// Check if any subscription matches our criteria
			for _, subscription := range subscriptions {
				if dm.matchesArtifactAndEvent(subscription, artifact, eventType) {
					matches = append(matches, SubscriptionMatch{
						Repository:   repoName,
						Subscription: subscription,
						RepoPath:     mainBranchPath,
					})
				}
			}
		}
	}

	// Sort matches alphabetically by repository name for deterministic behavior
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Repository < matches[j].Repository
	})

	return matches, nil
}

// LoadSubscriptions loads subscriptions from a repository's tako.yml file.
func (dm *DiscoveryManager) LoadSubscriptions(repoPath string) ([]config.Subscription, error) {
	takoYmlPath := filepath.Join(repoPath, "tako.yml")

	// Check if tako.yml exists
	if _, err := os.Stat(takoYmlPath); os.IsNotExist(err) {
		// No tako.yml file - this is not an error, just return empty subscriptions
		return []config.Subscription{}, nil
	}

	// Load the configuration
	cfg, err := config.Load(takoYmlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load tako.yml from %s: %v", takoYmlPath, err)
	}

	return cfg.Subscriptions, nil
}

// matchesArtifactAndEvent checks if a subscription matches the specified artifact and event type.
func (dm *DiscoveryManager) matchesArtifactAndEvent(subscription config.Subscription, artifact, eventType string) bool {
	// Check if the subscription's artifact matches
	if subscription.Artifact != artifact {
		return false
	}

	// Check if the subscription includes the specified event type
	for _, subEventType := range subscription.Events {
		if subEventType == eventType {
			return true
		}
	}

	return false
}

// GetRepositoryPath returns the local path for a cached repository.
// This method constructs the path based on Tako's caching convention.
func (dm *DiscoveryManager) GetRepositoryPath(owner, repo, branch string) string {
	if branch == "" {
		branch = "main" // Default to main branch
	}
	return filepath.Join(dm.cacheDir, "repos", owner, repo, branch)
}

// ScanRepositories returns a list of all cached repositories.
// Useful for debugging and administrative operations.
func (dm *DiscoveryManager) ScanRepositories() ([]string, error) {
	var repositories []string

	repoBaseDir := filepath.Join(dm.cacheDir, "repos")
	if _, err := os.Stat(repoBaseDir); os.IsNotExist(err) {
		return repositories, nil
	}

	// Walk through owner directories
	ownerEntries, err := os.ReadDir(repoBaseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache directory: %v", err)
	}

	for _, ownerEntry := range ownerEntries {
		if !ownerEntry.IsDir() {
			continue
		}

		ownerPath := filepath.Join(repoBaseDir, ownerEntry.Name())

		// Walk through repo directories
		repoEntries, err := os.ReadDir(ownerPath)
		if err != nil {
			continue
		}

		for _, repoEntry := range repoEntries {
			if !repoEntry.IsDir() {
				continue
			}

			repoName := fmt.Sprintf("%s/%s", ownerEntry.Name(), repoEntry.Name())
			repositories = append(repositories, repoName)
		}
	}

	// Sort for consistent output
	sort.Strings(repositories)
	return repositories, nil
}
