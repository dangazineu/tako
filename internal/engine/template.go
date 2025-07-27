package engine

import (
	"bytes"
	"container/list"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"
)

// TemplateEngine provides secure template processing with event context and caching.
type TemplateEngine struct {
	cache     *templateCache
	functions template.FuncMap
	mu        sync.RWMutex
}

// TemplateContext represents the complete context available in templates.
type TemplateContext struct {
	Inputs  map[string]string            `json:"inputs"`
	Steps   map[string]map[string]string `json:"steps"`
	Event   *EventContext                `json:"event,omitempty"`
	Trigger *TriggerContext              `json:"trigger,omitempty"` // Legacy compatibility
}

// EventContext provides event-specific data for subscription-triggered workflows.
type EventContext struct {
	Type      string                 `json:"type"`
	Payload   map[string]interface{} `json:"payload"`
	Source    string                 `json:"source"`
	Timestamp time.Time              `json:"timestamp"`
	Version   string                 `json:"version,omitempty"`
}

// TriggerContext provides legacy compatibility for artifact triggers.
type TriggerContext struct {
	Artifacts []ArtifactInfo `json:"artifacts"`
}

// ArtifactInfo represents information about a triggered artifact.
type ArtifactInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
}

// templateCacheEntry represents a cached template with metadata.
type templateCacheEntry struct {
	template  *template.Template
	size      int64
	createdAt time.Time
	accessAt  time.Time
}

// templateCache implements an LRU cache for parsed templates.
type templateCache struct {
	entries   map[string]*list.Element
	lru       *list.List
	maxSize   int64
	totalSize int64
	mu        sync.RWMutex
}

// NewTemplateEngine creates a new template engine with caching and security functions.
func NewTemplateEngine() *TemplateEngine {
	engine := &TemplateEngine{
		cache: newTemplateCache(100 * 1024 * 1024), // 100MB cache
	}

	// Initialize security and utility functions
	engine.functions = template.FuncMap{
		// Security functions
		"shell_quote": shellQuote,
		"json_escape": jsonEscape,
		"url_encode":  urlEncode,
		"html_escape": htmlEscape,

		// Event processing functions
		"event_field":     eventField,
		"event_has_field": eventHasField,
		"event_filter":    eventFilter,

		// Utility functions
		"default":    defaultValue,
		"empty":      isEmpty,
		"trim":       strings.TrimSpace,
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"split":      strings.Split,
		"join":       strings.Join,
		"replace":    strings.ReplaceAll,
		"contains":   strings.Contains,
		"has_prefix": strings.HasPrefix,
		"has_suffix": strings.HasSuffix,

		// Type conversion functions
		"to_string": toString,
		"to_int":    toInt,
		"to_float":  toFloat,
		"to_bool":   toBool,

		// Collection functions
		"range_map": rangeMap,
		"keys":      keys,
		"values":    values,
		"length":    length,
		"first":     first,
		"last":      last,

		// Conditional functions
		"if_then_else": ifThenElse,
		"or":           or,
		"and":          and,
		"not":          not,
	}

	return engine
}

// ExpandTemplate processes a template string with the provided context.
func (te *TemplateEngine) ExpandTemplate(tmplStr string, context *TemplateContext) (string, error) {
	if tmplStr == "" {
		return "", nil
	}

	// Check cache first
	tmpl, err := te.getOrCreateTemplate(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	// Execute template with context
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, context); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	return buf.String(), nil
}

// getOrCreateTemplate retrieves a cached template or creates and caches a new one.
func (te *TemplateEngine) getOrCreateTemplate(tmplStr string) (*template.Template, error) {
	te.mu.RLock()
	if tmpl := te.cache.get(tmplStr); tmpl != nil {
		te.mu.RUnlock()
		return tmpl, nil
	}
	te.mu.RUnlock()

	// Template not in cache, create new one
	te.mu.Lock()
	defer te.mu.Unlock()

	// Double-check after acquiring write lock
	if tmpl := te.cache.get(tmplStr); tmpl != nil {
		return tmpl, nil
	}

	// Create new template with security functions
	tmpl, err := template.New("template").Funcs(te.functions).Parse(tmplStr)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %v", err)
	}

	// Cache the template
	te.cache.put(tmplStr, tmpl)

	return tmpl, nil
}

// ValidateTemplate validates a template string without executing it.
func (te *TemplateEngine) ValidateTemplate(tmplStr string) error {
	_, err := te.getOrCreateTemplate(tmplStr)
	return err
}

// ClearCache clears the template cache.
func (te *TemplateEngine) ClearCache() {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.cache.clear()
}

// GetCacheStats returns statistics about the template cache.
func (te *TemplateEngine) GetCacheStats() map[string]interface{} {
	te.mu.RLock()
	defer te.mu.RUnlock()

	return map[string]interface{}{
		"entries":    len(te.cache.entries),
		"total_size": te.cache.totalSize,
		"max_size":   te.cache.maxSize,
	}
}

// newTemplateCache creates a new LRU template cache.
func newTemplateCache(maxSize int64) *templateCache {
	return &templateCache{
		entries: make(map[string]*list.Element),
		lru:     list.New(),
		maxSize: maxSize,
	}
}

// get retrieves a template from the cache.
func (tc *templateCache) get(key string) *template.Template {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if elem, exists := tc.entries[key]; exists {
		tc.lru.MoveToFront(elem)
		entry := elem.Value.(*templateCacheEntry)
		entry.accessAt = time.Now()
		return entry.template
	}
	return nil
}

// put adds a template to the cache.
func (tc *templateCache) put(key string, tmpl *template.Template) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Calculate approximate size (template string length)
	size := int64(len(key))

	entry := &templateCacheEntry{
		template:  tmpl,
		size:      size,
		createdAt: time.Now(),
		accessAt:  time.Now(),
	}

	// If entry already exists, update it
	if elem, exists := tc.entries[key]; exists {
		tc.lru.MoveToFront(elem)
		oldEntry := elem.Value.(*templateCacheEntry)
		tc.totalSize -= oldEntry.size
		elem.Value = entry
		tc.totalSize += size
		return
	}

	// Add new entry
	elem := tc.lru.PushFront(entry)
	tc.entries[key] = elem
	tc.totalSize += size

	// Evict entries if cache is too large
	tc.evictIfNeeded()
}

// evictIfNeeded removes least recently used entries if cache exceeds size limit.
func (tc *templateCache) evictIfNeeded() {
	for tc.totalSize > tc.maxSize && tc.lru.Len() > 0 {
		elem := tc.lru.Back()
		tc.lru.Remove(elem)
		entry := elem.Value.(*templateCacheEntry)
		tc.totalSize -= entry.size

		// Find and remove from entries map
		for key, e := range tc.entries {
			if e == elem {
				delete(tc.entries, key)
				break
			}
		}
	}
}

// clear removes all entries from the cache.
func (tc *templateCache) clear() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.entries = make(map[string]*list.Element)
	tc.lru = list.New()
	tc.totalSize = 0
}
