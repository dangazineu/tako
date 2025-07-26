package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Subscription represents a repository's subscription to events from other repositories.
type Subscription struct {
	Artifact      string            `yaml:"artifact"`                 // Format: repo:artifact (e.g., "my-org/go-lib:go-lib")
	Events        []string          `yaml:"events"`                   // List of event types to subscribe to
	SchemaVersion string            `yaml:"schema_version,omitempty"` // Compatible schema version range
	Filters       []string          `yaml:"filters,omitempty"`        // CEL expressions for event filtering
	Workflow      string            `yaml:"workflow"`                 // Workflow to trigger
	Inputs        map[string]string `yaml:"inputs,omitempty"`         // Input mappings for the triggered workflow
}

// validateArtifactReference validates the repo:artifact format.
func validateArtifactReference(artifact string) error {
	if artifact == "" {
		return fmt.Errorf("artifact reference cannot be empty")
	}

	// Expected format: owner/repo:artifact
	parts := strings.Split(artifact, ":")
	if len(parts) != 2 {
		return fmt.Errorf("artifact reference '%s' must be in format 'repo:artifact'", artifact)
	}

	repo := parts[0]
	artifactName := parts[1]

	// Validate repository format (owner/repo)
	repoParts := strings.Split(repo, "/")
	if len(repoParts) != 2 {
		return fmt.Errorf("repository '%s' must be in format 'owner/repo'", repo)
	}

	if repoParts[0] == "" || repoParts[1] == "" {
		return fmt.Errorf("repository '%s' has empty owner or repo name", repo)
	}

	// Validate artifact name (basic validation)
	if artifactName == "" {
		return fmt.Errorf("artifact name cannot be empty in reference '%s'", artifact)
	}

	// Artifact names should follow basic naming conventions
	matched, err := regexp.MatchString("^[a-zA-Z][a-zA-Z0-9_-]*$", artifactName)
	if err != nil {
		return fmt.Errorf("error validating artifact name: %w", err)
	}
	if !matched {
		return fmt.Errorf("artifact name '%s' must start with a letter and contain only letters, numbers, underscores, and hyphens", artifactName)
	}

	return nil
}

// ValidateSubscription validates a single subscription.
func (s *Subscription) ValidateSubscription() error {
	// Validate artifact reference
	if err := validateArtifactReference(s.Artifact); err != nil {
		return fmt.Errorf("invalid artifact reference: %w", err)
	}

	// Validate events list
	if len(s.Events) == 0 {
		return fmt.Errorf("events list cannot be empty")
	}

	for i, event := range s.Events {
		if err := validateEventType(event); err != nil {
			return fmt.Errorf("event %d: %w", i, err)
		}
	}

	// Validate schema version range using improved validation
	if err := validateSemverRange(s.SchemaVersion); err != nil {
		return fmt.Errorf("invalid schema version: %w", err)
	}

	// Validate CEL filters
	for i, filter := range s.Filters {
		if err := validateCELExpression(filter); err != nil {
			return fmt.Errorf("filter %d: %w", i, err)
		}
	}

	// Validate workflow name
	if s.Workflow == "" {
		return fmt.Errorf("workflow name cannot be empty")
	}

	// Basic workflow name validation
	matched, err := regexp.MatchString("^[a-zA-Z][a-zA-Z0-9_-]*$", s.Workflow)
	if err != nil {
		return fmt.Errorf("error validating workflow name: %w", err)
	}
	if !matched {
		return fmt.Errorf("workflow name '%s' must start with a letter and contain only letters, numbers, underscores, and hyphens", s.Workflow)
	}

	// Validate template expressions in input mappings
	for inputName, inputValue := range s.Inputs {
		if err := validateTemplateExpression(inputValue); err != nil {
			return fmt.Errorf("input '%s': %w", inputName, err)
		}
	}

	return nil
}

// ValidateSubscriptions validates a list of subscriptions.
func ValidateSubscriptions(subscriptions []Subscription) error {
	for i, subscription := range subscriptions {
		if err := subscription.ValidateSubscription(); err != nil {
			return fmt.Errorf("subscription %d: %w", i, err)
		}
	}

	return nil
}
