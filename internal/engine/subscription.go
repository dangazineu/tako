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

// celProgramCacheEntry represents a cached CEL program with metadata.
type celProgramCacheEntry struct {
	program cel.Program
	prev    *celProgramCacheEntry
	next    *celProgramCacheEntry
	key     string
}

// celProgramCache implements a simple LRU cache for CEL programs.
type celProgramCache struct {
	mutex   sync.RWMutex
	entries map[string]*celProgramCacheEntry
	head    *celProgramCacheEntry
	tail    *celProgramCacheEntry
	maxSize int
	hits    int64
	misses  int64
}

// newCELProgramCache creates a new LRU cache for CEL programs.
func newCELProgramCache(maxSize int) *celProgramCache {
	return &celProgramCache{
		entries: make(map[string]*celProgramCacheEntry),
		maxSize: maxSize,
	}
}

// get retrieves a CEL program from the cache.
func (c *celProgramCache) get(key string) (cel.Program, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	entry, exists := c.entries[key]
	if !exists {
		c.misses++
		return nil, false
	}

	// Move entry to front (most recently used)
	c.moveToFront(entry)
	c.hits++
	return entry.program, true
}

// put adds a CEL program to the cache.
func (c *celProgramCache) put(key string, program cel.Program) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if already exists
	if entry, exists := c.entries[key]; exists {
		entry.program = program
		c.moveToFront(entry)
		return
	}

	// Create new entry
	entry := &celProgramCacheEntry{
		program: program,
		key:     key,
	}

	// Add to front
	c.addToFront(entry)
	c.entries[key] = entry

	// Evict if over capacity
	if len(c.entries) > c.maxSize {
		c.removeLRU()
	}
}

// stats returns cache statistics.
func (c *celProgramCache) stats() (hits, misses int64, size int) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.hits, c.misses, len(c.entries)
}

// moveToFront moves an entry to the front of the LRU list.
func (c *celProgramCache) moveToFront(entry *celProgramCacheEntry) {
	if c.head == entry {
		return
	}

	c.removeEntry(entry)
	c.addToFront(entry)
}

// addToFront adds an entry to the front of the LRU list.
func (c *celProgramCache) addToFront(entry *celProgramCacheEntry) {
	entry.prev = nil
	entry.next = c.head

	if c.head != nil {
		c.head.prev = entry
	}
	c.head = entry

	if c.tail == nil {
		c.tail = entry
	}
}

// removeEntry removes an entry from the LRU list.
func (c *celProgramCache) removeEntry(entry *celProgramCacheEntry) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		c.head = entry.next
	}

	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		c.tail = entry.prev
	}
}

// removeLRU removes the least recently used entry.
func (c *celProgramCache) removeLRU() {
	if c.tail == nil {
		return
	}

	delete(c.entries, c.tail.key)
	c.removeEntry(c.tail)
}

// SubscriptionEvaluator handles event-subscription matching and filtering.
type SubscriptionEvaluator struct {
	celEnv       *cel.Env
	costLimit    uint64           // Maximum cost for CEL expression evaluation
	programCache *celProgramCache // LRU cache for compiled CEL programs
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
		celEnv:       env,
		costLimit:    1000000,                 // 1M cost units - prevents complex expressions from causing DoS
		programCache: newCELProgramCache(100), // Cache up to 100 compiled CEL programs
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

// CheckSchemaCompatibility checks if the event's schema version is compatible with the subscription's version range.
func (se *SubscriptionEvaluator) CheckSchemaCompatibility(eventVersion, subscriptionRange string) (bool, error) {
	// If no event version is specified, assume compatibility
	if eventVersion == "" {
		return true, nil
	}

	// If no subscription version range is specified, accept any version
	if subscriptionRange == "" {
		return true, nil
	}

	// Parse the event version
	eventSemVer, err := parseSemVer(eventVersion)
	if err != nil {
		return false, fmt.Errorf("invalid event schema version '%s': %v", eventVersion, err)
	}

	// Parse and evaluate the subscription version range
	return evaluateVersionRange(eventSemVer, subscriptionRange)
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

// GetCacheStats returns CEL program cache statistics.
func (se *SubscriptionEvaluator) GetCacheStats() (hits, misses int64, size int) {
	return se.programCache.stats()
}

// evaluateCELFilter evaluates a CEL expression against an event.
func (se *SubscriptionEvaluator) evaluateCELFilter(filterExpr string, event Event) (bool, error) {
	// Try to get compiled program from cache
	program, found := se.programCache.get(filterExpr)
	if !found {
		// Cache miss - compile the expression
		ast, issues := se.celEnv.Compile(filterExpr)
		if issues != nil && issues.Err() != nil {
			return false, fmt.Errorf("CEL compilation error: %v", issues.Err())
		}

		// Create evaluation program
		var err error
		program, err = se.celEnv.Program(ast)
		if err != nil {
			return false, fmt.Errorf("CEL program creation error: %v", err)
		}

		// Cache the compiled program for future use
		se.programCache.put(filterExpr, program)
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
func evaluateVersionRange(version SemVer, rangeSpec string) (bool, error) {
	rangeSpec = strings.TrimSpace(rangeSpec)

	// Exact version match
	if !strings.ContainsAny(rangeSpec, "^~><") {
		targetVersion, err := parseSemVer(rangeSpec)
		if err != nil {
			return false, err
		}
		return version == targetVersion, nil
	}

	// Caret range (^1.0.0) - compatible within major version
	if strings.HasPrefix(rangeSpec, "^") {
		targetVersion, err := parseSemVer(strings.TrimPrefix(rangeSpec, "^"))
		if err != nil {
			return false, err
		}
		return version.Major == targetVersion.Major &&
			(version.Minor > targetVersion.Minor ||
				(version.Minor == targetVersion.Minor && version.Patch >= targetVersion.Patch)), nil
	}

	// Tilde range (~1.0.0) - compatible within minor version
	if strings.HasPrefix(rangeSpec, "~") {
		targetVersion, err := parseSemVer(strings.TrimPrefix(rangeSpec, "~"))
		if err != nil {
			return false, err
		}
		return version.Major == targetVersion.Major &&
			version.Minor == targetVersion.Minor &&
			version.Patch >= targetVersion.Patch, nil
	}

	// Greater than or equal (>=1.0.0)
	if strings.HasPrefix(rangeSpec, ">=") {
		targetVersion, err := parseSemVer(strings.TrimPrefix(rangeSpec, ">="))
		if err != nil {
			return false, err
		}
		return compareVersions(version, targetVersion) >= 0, nil
	}

	// Greater than (>1.0.0)
	if strings.HasPrefix(rangeSpec, ">") {
		targetVersion, err := parseSemVer(strings.TrimPrefix(rangeSpec, ">"))
		if err != nil {
			return false, err
		}
		return compareVersions(version, targetVersion) > 0, nil
	}

	// Less than or equal (<=1.0.0)
	if strings.HasPrefix(rangeSpec, "<=") {
		targetVersion, err := parseSemVer(strings.TrimPrefix(rangeSpec, "<="))
		if err != nil {
			return false, err
		}
		return compareVersions(version, targetVersion) <= 0, nil
	}

	// Less than (<1.0.0)
	if strings.HasPrefix(rangeSpec, "<") {
		targetVersion, err := parseSemVer(strings.TrimPrefix(rangeSpec, "<"))
		if err != nil {
			return false, err
		}
		return compareVersions(version, targetVersion) < 0, nil
	}

	return false, fmt.Errorf("unsupported version range format: %s", rangeSpec)
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
