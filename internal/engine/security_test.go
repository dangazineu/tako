package engine

import (
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "safe string",
			input:    "hello_world-123",
			expected: "hello_world-123",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "''",
		},
		{
			name:     "string with spaces",
			input:    "hello world",
			expected: "'hello world'",
		},
		{
			name:     "string with single quote",
			input:    "don't",
			expected: "'don'\"'\"'t'",
		},
		{
			name:     "dangerous command",
			input:    "test'; rm -rf /; echo 'hacked",
			expected: "'test'\"'\"'; rm -rf /; echo '\"'\"'hacked'",
		},
		{
			name:     "integer input",
			input:    42,
			expected: "42",
		},
		{
			name:     "boolean input",
			input:    true,
			expected: "true",
		},
		{
			name:     "nil input",
			input:    nil,
			expected: "''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shellQuote(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestJSONEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "simple string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "string with quotes",
			input:    `hello "world"`,
			expected: `hello \"world\"`,
		},
		{
			name:     "string with backslashes",
			input:    `path\to\file`,
			expected: `path\\to\\file`,
		},
		{
			name:     "string with newlines",
			input:    "line1\nline2\r\nline3",
			expected: "line1\\nline2\\r\\nline3",
		},
		{
			name:     "string with tabs",
			input:    "col1\tcol2\tcol3",
			expected: "col1\\tcol2\\tcol3",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "integer input",
			input:    123,
			expected: "123",
		},
		{
			name:     "boolean input",
			input:    false,
			expected: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jsonEscape(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestURLEncode(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "simple string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "string with spaces",
			input:    "hello world",
			expected: "hello+world",
		},
		{
			name:     "string with special chars",
			input:    "hello & goodbye",
			expected: "hello+%26+goodbye",
		},
		{
			name:     "url with query params",
			input:    "https://example.com?param=value with spaces",
			expected: "https%3A%2F%2Fexample.com%3Fparam%3Dvalue+with+spaces",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "integer input",
			input:    42,
			expected: "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := urlEncode(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestHTMLEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "simple string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "html tags",
			input:    "<script>alert('xss')</script>",
			expected: "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
		},
		{
			name:     "html entities",
			input:    "Tom & Jerry",
			expected: "Tom &amp; Jerry",
		},
		{
			name:     "quotes",
			input:    `He said "Hello"`,
			expected: "He said &#34;Hello&#34;",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := htmlEscape(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTypeConversions(t *testing.T) {
	t.Run("toString", func(t *testing.T) {
		tests := []struct {
			input    interface{}
			expected string
		}{
			{"string", "string"},
			{42, "42"},
			{int64(42), "42"},
			{3.14, "3.14"},
			{true, "true"},
			{false, "false"},
			{nil, ""},
		}

		for _, tt := range tests {
			result := toString(tt.input)
			if result != tt.expected {
				t.Errorf("toString(%v): expected %q, got %q", tt.input, tt.expected, result)
			}
		}
	})

	t.Run("toInt", func(t *testing.T) {
		tests := []struct {
			input       interface{}
			expected    int
			shouldError bool
		}{
			{42, 42, false},
			{int64(42), 42, false},
			{3.14, 3, false},
			{"42", 42, false},
			{true, 1, false},
			{false, 0, false},
			{"invalid", 0, true},
		}

		for _, tt := range tests {
			result, err := toInt(tt.input)
			if tt.shouldError && err == nil {
				t.Errorf("toInt(%v): expected error but got none", tt.input)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("toInt(%v): unexpected error: %v", tt.input, err)
			}
			if !tt.shouldError && result != tt.expected {
				t.Errorf("toInt(%v): expected %d, got %d", tt.input, tt.expected, result)
			}
		}
	})

	t.Run("toBool", func(t *testing.T) {
		tests := []struct {
			input       interface{}
			expected    bool
			shouldError bool
		}{
			{true, true, false},
			{false, false, false},
			{1, true, false},
			{0, false, false},
			{3.14, true, false},
			{0.0, false, false},
			{"true", true, false},
			{"false", false, false},
			{"invalid", false, true},
		}

		for _, tt := range tests {
			result, err := toBool(tt.input)
			if tt.shouldError && err == nil {
				t.Errorf("toBool(%v): expected error but got none", tt.input)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("toBool(%v): unexpected error: %v", tt.input, err)
			}
			if !tt.shouldError && result != tt.expected {
				t.Errorf("toBool(%v): expected %t, got %t", tt.input, tt.expected, result)
			}
		}
	})
}

func TestUtilityFunctions(t *testing.T) {
	t.Run("defaultValue", func(t *testing.T) {
		tests := []struct {
			defaultVal interface{}
			value      interface{}
			expected   interface{}
		}{
			{"fallback", "", "fallback"},
			{"fallback", "value", "value"},
			{"fallback", nil, "fallback"},
			{42, 0, 42},
			{42, 10, 10},
		}

		for _, tt := range tests {
			result := defaultValue(tt.defaultVal, tt.value)
			if result != tt.expected {
				t.Errorf("defaultValue(%v, %v): expected %v, got %v", tt.defaultVal, tt.value, tt.expected, result)
			}
		}
	})

	t.Run("isEmpty", func(t *testing.T) {
		tests := []struct {
			input    interface{}
			expected bool
		}{
			{nil, true},
			{"", true},
			{"value", false},
			{0, true},
			{42, false},
			{false, true},
			{true, false},
			{[]interface{}{}, true},
			{[]interface{}{1, 2}, false},
			{map[string]interface{}{}, true},
			{map[string]interface{}{"key": "value"}, false},
		}

		for _, tt := range tests {
			result := isEmpty(tt.input)
			if result != tt.expected {
				t.Errorf("isEmpty(%v): expected %t, got %t", tt.input, tt.expected, result)
			}
		}
	})

	t.Run("length", func(t *testing.T) {
		tests := []struct {
			input    interface{}
			expected int
		}{
			{nil, 0},
			{"hello", 5},
			{[]interface{}{1, 2, 3}, 3},
			{map[string]interface{}{"a": 1, "b": 2}, 2},
			{42, 0}, // unsupported type
		}

		for _, tt := range tests {
			result := length(tt.input)
			if result != tt.expected {
				t.Errorf("length(%v): expected %d, got %d", tt.input, tt.expected, result)
			}
		}
	})
}

func TestCollectionFunctions(t *testing.T) {
	t.Run("keys", func(t *testing.T) {
		input := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		}
		result := keys(input)
		if len(result) != 3 {
			t.Errorf("Expected 3 keys, got %d", len(result))
		}

		// Check all keys are present (order doesn't matter)
		for _, key := range []string{"key1", "key2", "key3"} {
			found := false
			for _, k := range result {
				if k == key {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Key %s not found in result", key)
			}
		}
	})

	t.Run("values", func(t *testing.T) {
		input := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		}
		result := values(input)
		if len(result) != 2 {
			t.Errorf("Expected 2 values, got %d", len(result))
		}
	})

	t.Run("first", func(t *testing.T) {
		tests := []struct {
			input    interface{}
			expected interface{}
		}{
			{[]interface{}{"a", "b", "c"}, "a"},
			{"hello", "h"},
			{[]interface{}{}, nil},
			{"", nil},
		}

		for _, tt := range tests {
			result := first(tt.input)
			if result != tt.expected {
				t.Errorf("first(%v): expected %v, got %v", tt.input, tt.expected, result)
			}
		}
	})

	t.Run("last", func(t *testing.T) {
		tests := []struct {
			input    interface{}
			expected interface{}
		}{
			{[]interface{}{"a", "b", "c"}, "c"},
			{"hello", "o"},
			{[]interface{}{}, nil},
			{"", nil},
		}

		for _, tt := range tests {
			result := last(tt.input)
			if result != tt.expected {
				t.Errorf("last(%v): expected %v, got %v", tt.input, tt.expected, result)
			}
		}
	})
}

func TestLogicalFunctions(t *testing.T) {
	t.Run("isTruthy", func(t *testing.T) {
		tests := []struct {
			input    interface{}
			expected bool
		}{
			{nil, false},
			{true, true},
			{false, false},
			{"", false},
			{"value", true},
			{0, false},
			{42, true},
			{[]interface{}{}, false},
			{[]interface{}{1}, true},
			{map[string]interface{}{}, false},
			{map[string]interface{}{"key": "value"}, true},
		}

		for _, tt := range tests {
			result := isTruthy(tt.input)
			if result != tt.expected {
				t.Errorf("isTruthy(%v): expected %t, got %t", tt.input, tt.expected, result)
			}
		}
	})

	t.Run("ifThenElse", func(t *testing.T) {
		tests := []struct {
			condition interface{}
			trueVal   interface{}
			falseVal  interface{}
			expected  interface{}
		}{
			{true, "yes", "no", "yes"},
			{false, "yes", "no", "no"},
			{1, "yes", "no", "yes"},
			{0, "yes", "no", "no"},
			{"", "yes", "no", "no"},
			{"value", "yes", "no", "yes"},
		}

		for _, tt := range tests {
			result := ifThenElse(tt.condition, tt.trueVal, tt.falseVal)
			if result != tt.expected {
				t.Errorf("ifThenElse(%v, %v, %v): expected %v, got %v",
					tt.condition, tt.trueVal, tt.falseVal, tt.expected, result)
			}
		}
	})

	t.Run("logical operators", func(t *testing.T) {
		if !and(true, true) {
			t.Error("and(true, true) should be true")
		}
		if and(true, false) {
			t.Error("and(true, false) should be false")
		}
		if !or(true, false) {
			t.Error("or(true, false) should be true")
		}
		if or(false, false) {
			t.Error("or(false, false) should be false")
		}
		if !not(false) {
			t.Error("not(false) should be true")
		}
		if not(true) {
			t.Error("not(true) should be false")
		}
	})
}

func TestSecurityIntegration(t *testing.T) {
	// Test that security functions prevent common injection attacks

	t.Run("command injection prevention", func(t *testing.T) {
		maliciousInput := "normal'; rm -rf /; echo 'pwned"
		quoted := shellQuote(maliciousInput)

		// Should start and end with quotes (proper shell quoting)
		if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
			t.Error("Shell quoting should wrap the string in quotes")
		}

		// Should contain the escaped version of single quotes
		if !strings.Contains(quoted, "'\"'\"'") {
			t.Error("Shell quoting didn't properly escape single quotes")
		}

		// The dangerous command should be rendered harmless (inside quotes)
		// We verify this by checking that the quoted string can be safely used in a shell command
		if !strings.Contains(quoted, "rm -rf /") {
			t.Error("Test setup issue: expected malicious content to be present but quoted")
		}
	})

	t.Run("XSS prevention", func(t *testing.T) {
		maliciousInput := "<script>alert('xss')</script>"
		escaped := htmlEscape(maliciousInput)

		// Should not contain actual HTML tags
		if strings.Contains(escaped, "<script>") {
			t.Error("HTML escaping failed to prevent XSS")
		}

		// Should contain escaped version
		if !strings.Contains(escaped, "&lt;script&gt;") {
			t.Error("HTML escaping didn't properly escape tags")
		}
	})

	t.Run("JSON injection prevention", func(t *testing.T) {
		maliciousInput := `","admin":true,"fake":"`
		escaped := jsonEscape(maliciousInput)

		// Should not contain unescaped quotes that could break JSON structure
		if strings.Contains(escaped, `","admin":true`) {
			t.Error("JSON escaping failed to prevent injection")
		}

		// Should contain escaped quotes
		if !strings.Contains(escaped, `\",\"`) {
			t.Error("JSON escaping didn't properly escape quotes")
		}
	})
}
