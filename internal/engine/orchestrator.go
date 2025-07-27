package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dangazineu/tako/internal/config"
	"github.com/dangazineu/tako/internal/interfaces"
)

// Orchestrator manages multi-repository workflow orchestration through event-driven subscriptions.
type Orchestrator struct {
	workspaceRoot string
	cacheDir      string
}

// Orchestrator implements SubscriptionDiscoverer interface.
var _ interfaces.SubscriptionDiscoverer = (*Orchestrator)(nil)

// NewOrchestrator creates a new orchestrator for multi-repository coordination.
func NewOrchestrator(workspaceRoot, cacheDir string) *Orchestrator {
	return &Orchestrator{
		workspaceRoot: workspaceRoot,
		cacheDir:      cacheDir,
	}
}

// DiscoverSubscriptions finds all repositories with subscriptions matching the given event.
func (o *Orchestrator) DiscoverSubscriptions(ctx context.Context, eventType, artifactRef string, eventPayload map[string]string) ([]interfaces.SubscriptionMatch, error) {
	repositories, err := o.discoverRepositories(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to discover repositories: %w", err)
	}

	var matches []interfaces.SubscriptionMatch
	for repoPath, repoName := range repositories {
		repoMatches, err := o.evaluateRepositorySubscriptions(ctx, repoPath, repoName, eventType, artifactRef, eventPayload)
		if err != nil {
			// Log error but continue with other repositories
			continue
		}
		matches = append(matches, repoMatches...)
	}

	return matches, nil
}

// discoverRepositories scans the cache directory for repositories with tako.yml files.
func (o *Orchestrator) discoverRepositories(_ context.Context) (map[string]string, error) {
	repositories := make(map[string]string)

	// Use cache directory structure: ~/.tako/cache/repos/<owner>/<repo>/<branch>
	reposDir := filepath.Join(o.cacheDir, "repos")
	if _, err := os.Stat(reposDir); os.IsNotExist(err) {
		return repositories, nil
	}

	err := filepath.Walk(reposDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Look for tako.yml files
		if info.Name() == "tako.yml" || info.Name() == "tako.yaml" {
			repoPath := filepath.Dir(path)
			repoName := o.extractRepositoryName(repoPath)
			if repoName != "" {
				repositories[repoPath] = repoName
			}
		}

		return nil
	})

	return repositories, err
}

// extractRepositoryName extracts owner/repo from cache path structure.
func (o *Orchestrator) extractRepositoryName(repoPath string) string {
	// Expected structure: ~/.tako/cache/repos/<owner>/<repo>/<branch>
	parts := strings.Split(repoPath, string(os.PathSeparator))
	if len(parts) < 3 {
		return ""
	}

	// Find "repos" in the path and extract owner/repo from following parts
	repoIndex := -1
	for i, part := range parts {
		if part == "repos" {
			repoIndex = i
			break
		}
	}

	if repoIndex == -1 || repoIndex+2 >= len(parts) {
		return ""
	}

	owner := parts[repoIndex+1]
	repo := parts[repoIndex+2]

	if owner == "" || repo == "" {
		return ""
	}

	return fmt.Sprintf("%s/%s", owner, repo)
}

// evaluateRepositorySubscriptions checks if a repository has subscriptions matching the event.
func (o *Orchestrator) evaluateRepositorySubscriptions(_ context.Context, repoPath, repoName, eventType, artifactRef string, eventPayload map[string]string) ([]interfaces.SubscriptionMatch, error) {
	// Load repository configuration
	configPath := filepath.Join(repoPath, "tako.yml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Try tako.yaml as fallback
		configPath = filepath.Join(repoPath, "tako.yaml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("no tako configuration found in %s", repoPath)
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", configPath, err)
	}

	var matches []interfaces.SubscriptionMatch
	for _, subscription := range cfg.Subscriptions {
		if o.subscriptionMatches(subscription, eventType, artifactRef, eventPayload) {
			matches = append(matches, interfaces.SubscriptionMatch{
				RepositoryPath: repoPath,
				RepositoryName: repoName,
				Subscription:   subscription,
				Config:         cfg,
			})
		}
	}

	return matches, nil
}

// subscriptionMatches evaluates if a subscription matches the given event and artifact.
func (o *Orchestrator) subscriptionMatches(subscription config.Subscription, eventType, artifactRef string, _ map[string]string) bool {
	// Check artifact reference match
	if subscription.Artifact != artifactRef {
		return false
	}

	// Check event type match
	eventMatches := false
	for _, subEventType := range subscription.Events {
		if subEventType == eventType {
			eventMatches = true
			break
		}
	}
	if !eventMatches {
		return false
	}

	// TODO: Implement schema version compatibility checking
	// For now, we'll skip schema version validation

	// TODO: Implement CEL filter evaluation
	// For now, we'll skip filter evaluation

	return true
}

// ValidateSchemaCompatibility checks if the event schema version is compatible with subscription requirements.
func (o *Orchestrator) ValidateSchemaCompatibility(eventSchemaVersion, subscriptionSchemaVersion string) bool {
	// If no version specified in subscription, accept any event version
	if subscriptionSchemaVersion == "" {
		return true
	}

	// If no version in event, only match if subscription also has no version requirement
	if eventSchemaVersion == "" {
		return subscriptionSchemaVersion == ""
	}

	// For now, implement exact version matching
	// TODO: Implement proper semver range matching
	return eventSchemaVersion == subscriptionSchemaVersion
}

// EvaluateCELFilter evaluates a CEL expression against event context.
func (o *Orchestrator) EvaluateCELFilter(filter string, eventContext map[string]interface{}) (bool, error) {
	// For now, return true to accept all events
	// TODO: Implement proper CEL evaluation with event context
	return true, nil
}
