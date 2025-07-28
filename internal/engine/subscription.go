package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/dangazineu/tako/internal/config"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

// Event represents an event emitted by a fan-out step.
type Event struct {
	Type          string
	SchemaVersion string
	Payload       map[string]interface{}
	Source        string
	Timestamp     int64
}

// CompiledCELProgram represents a cached, compiled CEL program.
type CompiledCELProgram struct {
	program cel.Program
	ast     *cel.Ast
}

// SubscriptionEvaluator handles event-subscription matching and filtering.
type SubscriptionEvaluator struct {
	celEnv       *cel.Env
	costLimit    uint64       // Maximum cost for CEL expression evaluation
	programCache sync.Map     // Thread-safe cache for compiled CEL programs
	cacheLimit   int          // Maximum number of cached programs
	cacheSize    int64        // Current cache size (approximate)
	cacheMutex   sync.RWMutex // Protects cache metadata
}

// NewSubscriptionEvaluator creates a new subscription evaluator with security safeguards.
func NewSubscriptionEvaluator() (*SubscriptionEvaluator, error) {
	// Create CEL environment with security constraints
	env, err := cel.NewEnv(
		cel.Variable("event", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("payload", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("event_type", cel.StringType),
		cel.Variable("schema_version", cel.StringType),
		cel.Variable("source", cel.StringType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %v", err)
	}

	return &SubscriptionEvaluator{
		celEnv:     env,
		costLimit:  1000000, // 1M cost units - prevents complex expressions from causing DoS
		cacheLimit: 1000,    // Maximum 1000 cached CEL programs
	}, nil
}

// EvaluateSubscription checks if a subscription matches the specified event.
func (se *SubscriptionEvaluator) EvaluateSubscription(subscription config.Subscription, event Event) (bool, error) {
	// First check basic event type matching
	eventTypeMatches := false
	for _, subEventType := range subscription.Events {
		if subEventType == event.Type {
			eventTypeMatches = true
			break
		}
	}
	if !eventTypeMatches {
		return false, nil
	}

	// Check schema version compatibility if specified
	if subscription.SchemaVersion != "" {
		compatible, err := se.CheckSchemaCompatibility(event.SchemaVersion, subscription.SchemaVersion)
		if err != nil {
			return false, fmt.Errorf("schema compatibility check failed: %v", err)
		}
		if !compatible {
			return false, nil
		}
	}

	// Evaluate CEL filter expressions if present
	for i, filter := range subscription.Filters {
		matches, err := se.evaluateCELFilter(filter, event)
		if err != nil {
			return false, fmt.Errorf("filter %d evaluation failed: %v", i, err)
		}
		if !matches {
			return false, nil
		}
	}

	return true, nil
}

// SchemaCompatibilityResult represents the result of a schema compatibility check.
type SchemaCompatibilityResult struct {
	Compatible bool
	Reason     string // Human-readable explanation of compatibility or incompatibility
	Details    string // Additional technical details
}

// CheckSchemaCompatibility checks if the event's schema version is compatible with the subscription's version range.
func (se *SubscriptionEvaluator) CheckSchemaCompatibility(eventVersion, subscriptionRange string) (bool, error) {
	result := se.CheckSchemaCompatibilityDetailed(eventVersion, subscriptionRange)

	// Only return errors for parsing/validation issues, not for valid but incompatible versions
	if !result.Compatible {
		// Check if this is a validation error (invalid format) vs compatibility issue (valid format but incompatible)
		if stringContains(result.Reason, "Invalid") && (stringContains(result.Reason, "schema version") || stringContains(result.Reason, "range")) {
			return false, fmt.Errorf("schema compatibility failed: %s", result.Reason)
		}
		// For valid but incompatible versions, return false without error
		return false, nil
	}

	return result.Compatible, nil
}

// CheckSchemaCompatibilityDetailed provides detailed schema compatibility checking with comprehensive error reporting.
func (se *SubscriptionEvaluator) CheckSchemaCompatibilityDetailed(eventVersion, subscriptionRange string) SchemaCompatibilityResult {
	// If no event version is specified, assume compatibility
	if eventVersion == "" {
		return SchemaCompatibilityResult{
			Compatible: true,
			Reason:     "Event has no schema version specified (backward compatibility)",
			Details:    "Events without schema versions are accepted for backward compatibility",
		}
	}

	// If no subscription version range is specified, accept any version
	if subscriptionRange == "" {
		return SchemaCompatibilityResult{
			Compatible: true,
			Reason:     "Subscription accepts any schema version",
			Details:    "No version constraint specified in subscription",
		}
	}

	// Parse the event version
	eventSemVer, err := parseSemVer(eventVersion)
	if err != nil {
		return SchemaCompatibilityResult{
			Compatible: false,
			Reason:     fmt.Sprintf("Invalid event schema version '%s': %v", eventVersion, err),
			Details:    "Event schema version must follow semantic versioning (major.minor.patch)",
		}
	}

	// Parse and evaluate the subscription version range
	compatible, reason, details := evaluateVersionRangeDetailed(eventSemVer, subscriptionRange)
	return SchemaCompatibilityResult{
		Compatible: compatible,
		Reason:     reason,
		Details:    details,
	}
}

// ProcessEventPayload processes the event payload for input mapping to workflow inputs.
func (se *SubscriptionEvaluator) ProcessEventPayload(payload map[string]interface{}, subscription config.Subscription) (map[string]string, error) {
	result := make(map[string]string)

	// Process each input mapping in the subscription
	for inputName, inputValue := range subscription.Inputs {
		// For now, we'll do simple template variable substitution
		// This will be enhanced to use the full template engine in later phases
		processedValue, err := se.processSimpleTemplate(inputValue, payload)
		if err != nil {
			return nil, fmt.Errorf("failed to process input '%s': %v", inputName, err)
		}
		result[inputName] = processedValue
	}

	return result, nil
}

// evaluateCELFilter evaluates a CEL expression against an event using cached compiled programs.
func (se *SubscriptionEvaluator) evaluateCELFilter(filterExpr string, event Event) (bool, error) {
	// Get or compile the CEL program (with caching)
	program, err := se.getOrCompileCELProgram(filterExpr)
	if err != nil {
		return false, err
	}

	// Prepare evaluation context
	evalCtx := map[string]interface{}{
		"event":          eventToMap(event),
		"payload":        event.Payload,
		"event_type":     event.Type,
		"schema_version": event.SchemaVersion,
		"source":         event.Source,
	}

	// Evaluate the expression
	result, _, err := program.Eval(evalCtx)
	if err != nil {
		return false, fmt.Errorf("CEL evaluation error: %v", err)
	}

	// Convert result to boolean
	if result.Type() != types.BoolType {
		return false, fmt.Errorf("CEL expression must return boolean, got %v", result.Type())
	}

	return result.Value().(bool), nil
}

// getOrCompileCELProgram retrieves a compiled CEL program from cache or compiles and caches it.
func (se *SubscriptionEvaluator) getOrCompileCELProgram(filterExpr string) (cel.Program, error) {
	// Try to get from cache first
	if cached, found := se.programCache.Load(filterExpr); found {
		if compiledProgram, ok := cached.(*CompiledCELProgram); ok {
			return compiledProgram.program, nil
		}
	}

	// Not in cache, compile the expression
	ast, issues := se.celEnv.Compile(filterExpr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("CEL compilation error: %v", issues.Err())
	}

	// Create evaluation program
	program, err := se.celEnv.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("CEL program creation error: %v", err)
	}

	// Cache the compiled program (with cache size management)
	compiledProgram := &CompiledCELProgram{
		program: program,
		ast:     ast,
	}

	// Thread-safe cache management
	se.cacheMutex.Lock()
	defer se.cacheMutex.Unlock()

	// Double-check if it was added while we were waiting for the lock
	if cached, found := se.programCache.Load(filterExpr); found {
		if existingProgram, ok := cached.(*CompiledCELProgram); ok {
			return existingProgram.program, nil
		}
	}

	// Check cache size before adding
	if se.cacheSize < int64(se.cacheLimit) {
		se.programCache.Store(filterExpr, compiledProgram)
		se.cacheSize++
	} else {
		// Cache is full, implement LRU eviction by clearing cache
		// In a production implementation, this could be more sophisticated
		se.clearCacheUnsafe()
		se.programCache.Store(filterExpr, compiledProgram)
		se.cacheSize = 1
	}

	return program, nil
}

// clearCacheUnsafe clears the entire program cache. Must be called with cacheMutex held.
func (se *SubscriptionEvaluator) clearCacheUnsafe() {
	se.programCache.Range(func(key, value interface{}) bool {
		se.programCache.Delete(key)
		return true
	})
	se.cacheSize = 0
}

// ClearCache clears the CEL program cache. Useful for testing and memory management.
func (se *SubscriptionEvaluator) ClearCache() {
	se.cacheMutex.Lock()
	defer se.cacheMutex.Unlock()
	se.clearCacheUnsafe()
}

// GetCacheStats returns statistics about the CEL program cache.
func (se *SubscriptionEvaluator) GetCacheStats() (size int64, limit int) {
	se.cacheMutex.RLock()
	defer se.cacheMutex.RUnlock()
	return se.cacheSize, se.cacheLimit
}

// processSimpleTemplate processes a simple template string with variable substitution.
// This is a simplified implementation - full template processing will be added in later phases.
func (se *SubscriptionEvaluator) processSimpleTemplate(template string, payload map[string]interface{}) (string, error) {
	result := template

	// Simple variable substitution for {{ .payload.field }} patterns
	re := regexp.MustCompile(`\{\{\s*\.payload\.([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)
	matches := re.FindAllStringSubmatch(template, -1)

	for _, match := range matches {
		fullMatch := match[0]
		fieldName := match[1]

		if value, exists := payload[fieldName]; exists {
			// Convert value to string
			strValue := fmt.Sprintf("%v", value)
			result = strings.ReplaceAll(result, fullMatch, strValue)
		} else {
			return "", fmt.Errorf("payload field '%s' not found", fieldName)
		}
	}

	return result, nil
}

// eventToMap converts an Event to a map for CEL evaluation.
func eventToMap(event Event) map[string]interface{} {
	return map[string]interface{}{
		"type":           event.Type,
		"schema_version": event.SchemaVersion,
		"payload":        event.Payload,
		"source":         event.Source,
		"timestamp":      event.Timestamp,
	}
}

// SemVer represents a semantic version.
type SemVer struct {
	Major int
	Minor int
	Patch int
}

// parseSemVer parses a semantic version string.
func parseSemVer(version string) (SemVer, error) {
	// Basic semantic version regex (major.minor.patch)
	re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)
	matches := re.FindStringSubmatch(version)
	if len(matches) != 4 {
		return SemVer{}, fmt.Errorf("invalid semantic version format: %s", version)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid major version: %v", err)
	}

	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid minor version: %v", err)
	}

	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid patch version: %v", err)
	}

	return SemVer{Major: major, Minor: minor, Patch: patch}, nil
}

// evaluateVersionRange evaluates whether a semantic version satisfies a version range.
// Supports basic ranges like "1.0.0", "^1.0.0", "~1.0.0", ">=1.0.0".
// This is a legacy wrapper for backward compatibility.
func evaluateVersionRange(version SemVer, rangeSpec string) (bool, error) {
	compatible, reason, _ := evaluateVersionRangeDetailed(version, rangeSpec)

	// Only return errors for parsing/validation issues, not for valid but incompatible versions
	if !compatible && (stringContains(reason, "Invalid") || stringContains(reason, "Unsupported")) {
		return false, fmt.Errorf("%s", reason)
	}

	// For valid but incompatible versions, return false without error (legacy behavior)
	return compatible, nil
}

// evaluateVersionRangeDetailed evaluates whether a semantic version satisfies a version range with detailed error reporting.
// Returns: (compatible, reason, details).
func evaluateVersionRangeDetailed(version SemVer, rangeSpec string) (bool, string, string) {
	rangeSpec = strings.TrimSpace(rangeSpec)
	versionStr := fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)

	// Check for unsupported operators first
	if strings.Contains(rangeSpec, "!=") || strings.Contains(rangeSpec, "==") ||
		(strings.Contains(rangeSpec, "=") && !strings.Contains(rangeSpec, ">=") && !strings.Contains(rangeSpec, "<=")) {
		return false,
			fmt.Sprintf("Unsupported version range format: %s", rangeSpec),
			"Supported formats: exact (1.0.0), caret (^1.0.0), tilde (~1.0.0), comparison operators (>=1.0.0, >1.0.0, <=1.0.0, <1.0.0)"
	}

	// Exact version match
	if !strings.ContainsAny(rangeSpec, "^~><") {
		targetVersion, err := parseSemVer(rangeSpec)
		if err != nil {
			return false,
				fmt.Sprintf("Invalid version range '%s': %v", rangeSpec, err),
				"Version ranges must follow semantic versioning format (major.minor.patch)"
		}

		compatible := version == targetVersion
		if compatible {
			return true,
				fmt.Sprintf("Event version %s exactly matches required version %s", versionStr, rangeSpec),
				"Exact version match provides strongest compatibility guarantee"
		} else {
			return false,
				fmt.Sprintf("Event version %s does not match required version %s", versionStr, rangeSpec),
				"Exact version constraints require perfect match for compatibility"
		}
	}

	// Caret range (^1.0.0) - compatible within major version
	if strings.HasPrefix(rangeSpec, "^") {
		targetVersionStr := strings.TrimPrefix(rangeSpec, "^")
		targetVersion, err := parseSemVer(targetVersionStr)
		if err != nil {
			return false,
				fmt.Sprintf("Invalid caret range '%s': %v", rangeSpec, err),
				"Caret ranges must follow format ^major.minor.patch"
		}

		compatible := version.Major == targetVersion.Major &&
			(version.Minor > targetVersion.Minor ||
				(version.Minor == targetVersion.Minor && version.Patch >= targetVersion.Patch))

		if compatible {
			return true,
				fmt.Sprintf("Event version %s is compatible with caret range %s", versionStr, rangeSpec),
				fmt.Sprintf("Caret range allows minor and patch updates within major version %d", targetVersion.Major)
		} else {
			if version.Major != targetVersion.Major {
				return false,
					fmt.Sprintf("Event version %s has different major version than range %s (breaking changes)", versionStr, rangeSpec),
					"Caret ranges reject different major versions due to potential breaking changes"
			} else {
				return false,
					fmt.Sprintf("Event version %s is older than minimum version in range %s", versionStr, rangeSpec),
					"Caret ranges require version to be at least the specified version"
			}
		}
	}

	// Tilde range (~1.0.0) - compatible within minor version
	if strings.HasPrefix(rangeSpec, "~") {
		targetVersionStr := strings.TrimPrefix(rangeSpec, "~")
		targetVersion, err := parseSemVer(targetVersionStr)
		if err != nil {
			return false,
				fmt.Sprintf("Invalid tilde range '%s': %v", rangeSpec, err),
				"Tilde ranges must follow format ~major.minor.patch"
		}

		compatible := version.Major == targetVersion.Major &&
			version.Minor == targetVersion.Minor &&
			version.Patch >= targetVersion.Patch

		if compatible {
			return true,
				fmt.Sprintf("Event version %s is compatible with tilde range %s", versionStr, rangeSpec),
				fmt.Sprintf("Tilde range allows patch updates within version %d.%d", targetVersion.Major, targetVersion.Minor)
		} else {
			if version.Major != targetVersion.Major || version.Minor != targetVersion.Minor {
				return false,
					fmt.Sprintf("Event version %s has different major.minor than range %s", versionStr, rangeSpec),
					"Tilde ranges reject different major or minor versions"
			} else {
				return false,
					fmt.Sprintf("Event version %s is older than minimum patch in range %s", versionStr, rangeSpec),
					"Tilde ranges require patch version to be at least the specified version"
			}
		}
	}

	// Greater than or equal (>=1.0.0)
	if strings.HasPrefix(rangeSpec, ">=") {
		targetVersionStr := strings.TrimPrefix(rangeSpec, ">=")
		targetVersion, err := parseSemVer(targetVersionStr)
		if err != nil {
			return false,
				fmt.Sprintf("Invalid >= range '%s': %v", rangeSpec, err),
				">= ranges must follow format >=major.minor.patch"
		}

		comparison := compareVersions(version, targetVersion)
		compatible := comparison >= 0

		if compatible {
			return true,
				fmt.Sprintf("Event version %s satisfies >= constraint %s", versionStr, rangeSpec),
				"Greater-than-or-equal allows any version at or above the specified version"
		} else {
			return false,
				fmt.Sprintf("Event version %s is older than minimum required %s", versionStr, targetVersionStr),
				"Greater-than-or-equal constraint requires version to be at least the specified version"
		}
	}

	// Greater than (>1.0.0)
	if strings.HasPrefix(rangeSpec, ">") {
		targetVersionStr := strings.TrimPrefix(rangeSpec, ">")
		targetVersion, err := parseSemVer(targetVersionStr)
		if err != nil {
			return false,
				fmt.Sprintf("Invalid > range '%s': %v", rangeSpec, err),
				"> ranges must follow format >major.minor.patch"
		}

		comparison := compareVersions(version, targetVersion)
		compatible := comparison > 0

		if compatible {
			return true,
				fmt.Sprintf("Event version %s satisfies > constraint %s", versionStr, rangeSpec),
				"Greater-than allows any version newer than the specified version"
		} else {
			return false,
				fmt.Sprintf("Event version %s is not newer than required %s", versionStr, targetVersionStr),
				"Greater-than constraint requires version to be newer than the specified version"
		}
	}

	// Less than or equal (<=1.0.0)
	if strings.HasPrefix(rangeSpec, "<=") {
		targetVersionStr := strings.TrimPrefix(rangeSpec, "<=")
		targetVersion, err := parseSemVer(targetVersionStr)
		if err != nil {
			return false,
				fmt.Sprintf("Invalid <= range '%s': %v", rangeSpec, err),
				"<= ranges must follow format <=major.minor.patch"
		}

		comparison := compareVersions(version, targetVersion)
		compatible := comparison <= 0

		if compatible {
			return true,
				fmt.Sprintf("Event version %s satisfies <= constraint %s", versionStr, rangeSpec),
				"Less-than-or-equal allows any version at or below the specified version"
		} else {
			return false,
				fmt.Sprintf("Event version %s is newer than maximum allowed %s", versionStr, targetVersionStr),
				"Less-than-or-equal constraint requires version to be at most the specified version"
		}
	}

	// Less than (<1.0.0)
	if strings.HasPrefix(rangeSpec, "<") {
		targetVersionStr := strings.TrimPrefix(rangeSpec, "<")
		targetVersion, err := parseSemVer(targetVersionStr)
		if err != nil {
			return false,
				fmt.Sprintf("Invalid < range '%s': %v", rangeSpec, err),
				"< ranges must follow format <major.minor.patch"
		}

		comparison := compareVersions(version, targetVersion)
		compatible := comparison < 0

		if compatible {
			return true,
				fmt.Sprintf("Event version %s satisfies < constraint %s", versionStr, rangeSpec),
				"Less-than allows any version older than the specified version"
		} else {
			return false,
				fmt.Sprintf("Event version %s is not older than required %s", versionStr, targetVersionStr),
				"Less-than constraint requires version to be older than the specified version"
		}
	}

	return false,
		fmt.Sprintf("Unsupported version range format: %s", rangeSpec),
		"Supported formats: exact (1.0.0), caret (^1.0.0), tilde (~1.0.0), comparison operators (>=1.0.0, >1.0.0, <=1.0.0, <1.0.0)"
}

// GetSchemaEvolutionGuidelines provides comprehensive guidelines for schema version management and evolution.
func GetSchemaEvolutionGuidelines() map[string]string {
	return map[string]string{
		"semantic_versioning": "Use semantic versioning (major.minor.patch) for all schema versions. Increment MAJOR for breaking changes, MINOR for backward-compatible additions, PATCH for backward-compatible bug fixes.",

		"version_range_selection": "Choose version ranges based on compatibility requirements: exact versions (1.0.0) for strict compatibility, caret ranges (^1.0.0) for same major version compatibility, tilde ranges (~1.0.0) for same minor version compatibility.",

		"breaking_changes": "Breaking changes (field removal, type changes, renamed fields) MUST increment the major version. Examples: removing a required field, changing field types, restructuring the schema.",

		"compatible_changes": "Compatible changes (new optional fields, additional enum values) should increment the minor version. Examples: adding optional fields, adding new event types, extending existing structures.",

		"bug_fixes": "Bug fixes (documentation updates, constraint clarifications) should increment the patch version. Examples: fixing field descriptions, clarifying validation rules, correcting examples.",

		"subscription_strategies": "Producer-consumer version compatibility: Use caret ranges (^1.0.0) for flexible compatibility, exact versions for strict control, >= ranges for minimum version requirements.",

		"migration_planning": "When introducing breaking changes: document migration path, provide transition period with dual support, communicate changes to downstream consumers, consider backward compatibility shims.",

		"best_practices": "Schema evolution best practices: start with optional fields when possible, avoid field removal, use deprecation warnings before breaking changes, maintain comprehensive documentation, test compatibility across versions.",

		"validation_recommendations": "Validation strategy: validate events against declared schema versions, provide clear error messages for incompatible versions, log compatibility warnings for deprecated versions, support gradual migration periods.",

		"compatibility_matrix": "Version compatibility guide: 1.0.0 -> 1.1.0 (safe, new features), 1.0.0 -> 2.0.0 (breaking, requires migration), 1.1.0 -> 1.0.0 (unsafe, may use unsupported features), 2.0.0 -> 1.x.x (incompatible, different major version).",
	}
}

// compareVersions compares two semantic versions.
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2.
func compareVersions(v1, v2 SemVer) int {
	if v1.Major != v2.Major {
		if v1.Major < v2.Major {
			return -1
		}
		return 1
	}

	if v1.Minor != v2.Minor {
		if v1.Minor < v2.Minor {
			return -1
		}
		return 1
	}

	if v1.Patch != v2.Patch {
		if v1.Patch < v2.Patch {
			return -1
		}
		return 1
	}

	return 0
}

// stringContains checks if a string contains a substring (helper function).
func stringContains(s, substr string) bool {
	return strings.Contains(s, substr)
}
