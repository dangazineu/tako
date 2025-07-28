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
}

// NewTemplateEngine creates a new template engine with caching and security functions.
func NewTemplateEngine() *TemplateEngine {
	engine := &TemplateEngine{
		cache: newTemplateCache(100 * 1024 * 1024), // 100MB cache
	}

	// Initialize security and utility functions
	engine.functions = template.FuncMap{
		// Security functions (pipeline-compatible)
		"shell_quote": func(s interface{}) string {
			return shellQuote(toString(s))
		},
		"json_escape": func(s interface{}) string {
			return jsonEscape(toString(s))
		},
		"url_encode": func(s interface{}) string {
			return urlEncode(toString(s))
		},
		"html_escape": func(s interface{}) string {
			return htmlEscape(toString(s))
		},

		// Event processing functions
		"event_field":     eventField,
		"event_has_field": eventHasField,
		"event_filter":    eventFilter,

		// Utility functions (pipeline-compatible)
		"default": defaultValue,
		"empty":   isEmpty,
		"trim": func(s interface{}) string {
			return strings.TrimSpace(toString(s))
		},
		"upper": func(s interface{}) string {
			return strings.ToUpper(toString(s))
		},
		"lower": func(s interface{}) string {
			return strings.ToLower(toString(s))
		},
		"split": strings.Split,
		"join":  strings.Join,
		"replace": func(old, new string) func(string) string {
			return func(s string) string {
				return strings.ReplaceAll(s, old, new)
			}
		},
		"contains": func(substr string) func(string) bool {
			return func(s string) bool {
				return strings.Contains(s, substr)
			}
		},
		"has_prefix": func(prefix string) func(string) bool {
			return func(s string) bool {
				return strings.HasPrefix(s, prefix)
			}
		},
		"has_suffix": func(suffix string) func(string) bool {
			return func(s string) bool {
				return strings.HasSuffix(s, suffix)
			}
		},

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
	tc.entries = make(map[string]*list.Element)
	tc.lru = list.New()
	tc.totalSize = 0
}

// Security and utility function implementations

func shellQuote(s string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "'\"'\"'"))
}

func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

func urlEncode(s string) string {
	s = strings.ReplaceAll(s, "&", "%26")
	s = strings.ReplaceAll(s, "=", "%3D")
	s = strings.ReplaceAll(s, " ", "+")
	return s
}

func htmlEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, "&", "&amp;"), "<", "&lt;"), ">", "&gt;")
}

func defaultValue(def interface{}, val interface{}) interface{} {
	if val == nil || val == "" {
		return def
	}
	return val
}

func isEmpty(val interface{}) bool {
	if val == nil {
		return true
	}
	if s, ok := val.(string); ok {
		return s == ""
	}
	return false
}

func toString(val interface{}) string {
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%v", val)
}

func toInt(val interface{}) int {
	if s, ok := val.(string); ok {
		if i, err := fmt.Sscanf(s, "%d", new(int)); err == nil && i == 1 {
			var result int
			fmt.Sscanf(s, "%d", &result)
			return result
		}
	}
	return 0
}

func toFloat(val interface{}) float64 {
	if s, ok := val.(string); ok {
		if i, err := fmt.Sscanf(s, "%f", new(float64)); err == nil && i == 1 {
			var result float64
			fmt.Sscanf(s, "%f", &result)
			return result
		}
	}
	return 0.0
}

func toBool(val interface{}) bool {
	if s, ok := val.(string); ok {
		return s == "true" || s == "1" || s == "yes"
	}
	return false
}

func rangeMap(m map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(m))
	for k, v := range m {
		result = append(result, map[string]interface{}{"key": k, "value": v})
	}
	return result
}

func keys(m map[string]interface{}) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

func values(m map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}

func length(val interface{}) int {
	if s, ok := val.(string); ok {
		return len(s)
	}
	if m, ok := val.(map[string]interface{}); ok {
		return len(m)
	}
	if a, ok := val.([]interface{}); ok {
		return len(a)
	}
	return 0
}

func first(val interface{}) interface{} {
	if a, ok := val.([]interface{}); ok && len(a) > 0 {
		return a[0]
	}
	return nil
}

func last(val interface{}) interface{} {
	if a, ok := val.([]interface{}); ok && len(a) > 0 {
		return a[len(a)-1]
	}
	return nil
}

func ifThenElse(condition bool, thenVal, elseVal interface{}) interface{} {
	if condition {
		return thenVal
	}
	return elseVal
}

func or(vals ...interface{}) interface{} {
	for _, val := range vals {
		if val != nil && val != "" && val != false {
			return val
		}
	}
	return false
}

func and(vals ...interface{}) bool {
	for _, val := range vals {
		if val == nil || val == "" || val == false {
			return false
		}
	}
	return true
}

func not(val interface{}) bool {
	return val == nil || val == "" || val == false
}
