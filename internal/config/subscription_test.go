package config

import (
	"testing"
)

func TestValidateArtifactReference(t *testing.T) {
	testCases := []struct {
		name        string
		artifact    string
		expectError bool
	}{
		{"valid reference", "my-org/go-lib:go-lib", false},
		{"valid with hyphens", "my-org/client-app:main-service", false},
		{"valid with underscores", "my_org/go_lib:go_lib", false},
		{"empty string", "", true},
		{"missing colon", "my-org/go-lib", true},
		{"multiple colons", "my-org/go-lib:artifact:extra", true},
		{"empty repo", ":artifact", true},
		{"empty artifact", "my-org/go-lib:", true},
		{"invalid repo format", "my-org:go-lib", true},
		{"missing repo name", "my-org/:artifact", true},
		{"missing owner", "/go-lib:artifact", true},
		{"artifact starts with number", "my-org/go-lib:1artifact", true},
		{"artifact with spaces", "my-org/go-lib:my artifact", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateArtifactReference(tc.artifact)
			if tc.expectError && err == nil {
				t.Errorf("expected error for artifact %q, got nil", tc.artifact)
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error for artifact %q: %v", tc.artifact, err)
			}
		})
	}
}

func TestValidateSchemaVersionRange(t *testing.T) {
	testCases := []struct {
		name        string
		version     string
		expectError bool
	}{
		{"exact version", "1.2.3", false},
		{"caret range", "^1.2.3", false},
		{"tilde range", "~1.2.3", false},
		{"parenthesis range", "(1.1.0...2.0.0]", false},
		{"empty string (optional)", "", false},
		{"invalid format", "invalid", true},
		{"missing patch in exact", "1.2", true},
		{"invalid caret", "^1.2", true},
		{"invalid tilde", "~1.2", true},
		{"malformed range", "[1.1.0...2.0.0)", false},
		{"incomplete range", "(1.1.0...", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSemverRange(tc.version)
			if tc.expectError && err == nil {
				t.Errorf("expected error for version range %q, got nil", tc.version)
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error for version range %q: %v", tc.version, err)
			}
		})
	}
}

func TestSubscription_ValidateSubscription(t *testing.T) {
	testCases := []struct {
		name         string
		subscription Subscription
		expectError  bool
	}{
		{
			name: "valid subscription",
			subscription: Subscription{
				Artifact:      "my-org/go-lib:go-lib",
				Events:        []string{"library_built"},
				SchemaVersion: "^1.0.0",
				Workflow:      "update_integration",
			},
			expectError: false,
		},
		{
			name: "valid subscription without schema version",
			subscription: Subscription{
				Artifact: "my-org/go-lib:go-lib",
				Events:   []string{"library_built", "test_completed"},
				Workflow: "update_integration",
			},
			expectError: false,
		},
		{
			name: "invalid artifact reference",
			subscription: Subscription{
				Artifact: "invalid-format",
				Events:   []string{"library_built"},
				Workflow: "update_integration",
			},
			expectError: true,
		},
		{
			name: "empty events list",
			subscription: Subscription{
				Artifact: "my-org/go-lib:go-lib",
				Events:   []string{},
				Workflow: "update_integration",
			},
			expectError: true,
		},
		{
			name: "invalid event type",
			subscription: Subscription{
				Artifact: "my-org/go-lib:go-lib",
				Events:   []string{"invalid-event"},
				Workflow: "update_integration",
			},
			expectError: true,
		},
		{
			name: "invalid schema version",
			subscription: Subscription{
				Artifact:      "my-org/go-lib:go-lib",
				Events:        []string{"library_built"},
				SchemaVersion: "invalid",
				Workflow:      "update_integration",
			},
			expectError: true,
		},
		{
			name: "empty workflow name",
			subscription: Subscription{
				Artifact: "my-org/go-lib:go-lib",
				Events:   []string{"library_built"},
				Workflow: "",
			},
			expectError: true,
		},
		{
			name: "invalid workflow name",
			subscription: Subscription{
				Artifact: "my-org/go-lib:go-lib",
				Events:   []string{"library_built"},
				Workflow: "1invalid",
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.subscription.ValidateSubscription()
			if tc.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateSubscriptions(t *testing.T) {
	testCases := []struct {
		name          string
		subscriptions []Subscription
		expectError   bool
	}{
		{
			name: "valid subscriptions",
			subscriptions: []Subscription{
				{
					Artifact: "my-org/go-lib:go-lib",
					Events:   []string{"library_built"},
					Workflow: "update_integration",
				},
				{
					Artifact: "my-org/other-lib:other-lib",
					Events:   []string{"other_event"},
					Workflow: "other_workflow",
				},
			},
			expectError: false,
		},
		{
			name:          "empty subscriptions list",
			subscriptions: []Subscription{},
			expectError:   false,
		},
		{
			name: "one invalid subscription",
			subscriptions: []Subscription{
				{
					Artifact: "my-org/go-lib:go-lib",
					Events:   []string{"library_built"},
					Workflow: "update_integration",
				},
				{
					Artifact: "invalid-format",
					Events:   []string{"other_event"},
					Workflow: "other_workflow",
				},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSubscriptions(tc.subscriptions)
			if tc.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}