package engine

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// shellQuote safely quotes a string for use in shell commands.
// This prevents command injection by properly escaping shell metacharacters.
func shellQuote(s interface{}) string {
	str := toString(s)
	if str == "" {
		return "''"
	}

	// If the string contains only safe characters, return as-is
	safePat := regexp.MustCompile(`^[a-zA-Z0-9_./:-]+$`)
	if safePat.MatchString(str) {
		return str
	}

	// For other strings, use single quotes and escape any single quotes
	escaped := strings.ReplaceAll(str, "'", "'\"'\"'")
	return "'" + escaped + "'"
}

// jsonEscape escapes a string for safe inclusion in JSON.
func jsonEscape(s interface{}) string {
	str := toString(s)

	// Use Go's JSON marshaling for proper escaping
	data, err := json.Marshal(str)
	if err != nil {
		// Fallback to manual escaping
		return manualJSONEscape(str)
	}

	// Remove the surrounding quotes from JSON marshaling
	result := string(data)
	if len(result) >= 2 && result[0] == '"' && result[len(result)-1] == '"' {
		return result[1 : len(result)-1]
	}

	return result
}

// manualJSONEscape provides fallback JSON escaping.
func manualJSONEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\b", "\\b")
	s = strings.ReplaceAll(s, "\f", "\\f")
	return s
}

// urlEncode URL-encodes a string for safe inclusion in URLs.
func urlEncode(s interface{}) string {
	str := toString(s)
	return url.QueryEscape(str)
}

// htmlEscape escapes a string for safe inclusion in HTML.
func htmlEscape(s interface{}) string {
	str := toString(s)
	return html.EscapeString(str)
}

// toString converts various types to string safely.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}

// toInt converts various types to int safely.
func toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case int:
		return val, nil
	case int64:
		return int(val), nil
	case float64:
		return int(val), nil
	case string:
		return strconv.Atoi(val)
	case bool:
		if val {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

// toFloat converts various types to float64 safely.
func toFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		return strconv.ParseFloat(val, 64)
	case bool:
		if val {
			return 1.0, nil
		}
		return 0.0, nil
	default:
		return 0.0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// toBool converts various types to bool safely.
func toBool(v interface{}) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case int:
		return val != 0, nil
	case int64:
		return val != 0, nil
	case float64:
		return val != 0.0, nil
	case string:
		return strconv.ParseBool(val)
	default:
		return false, fmt.Errorf("cannot convert %T to bool", v)
	}
}

// defaultValue returns the default value if the input is empty/nil.
func defaultValue(defaultVal, value interface{}) interface{} {
	if isEmpty(value) {
		return defaultVal
	}
	return value
}

// isEmpty checks if a value is empty/nil.
func isEmpty(v interface{}) bool {
	if v == nil {
		return true
	}

	switch val := v.(type) {
	case string:
		return val == ""
	case []interface{}:
		return len(val) == 0
	case map[string]interface{}:
		return len(val) == 0
	case int:
		return val == 0
	case int64:
		return val == 0
	case float64:
		return val == 0.0
	case bool:
		return !val
	default:
		return false
	}
}

// length returns the length of various types.
func length(v interface{}) int {
	if v == nil {
		return 0
	}

	switch val := v.(type) {
	case string:
		return len(val)
	case []interface{}:
		return len(val)
	case map[string]interface{}:
		return len(val)
	default:
		return 0
	}
}

// first returns the first element of a slice or string.
func first(v interface{}) interface{} {
	switch val := v.(type) {
	case []interface{}:
		if len(val) > 0 {
			return val[0]
		}
	case string:
		if len(val) > 0 {
			return string(val[0])
		}
	}
	return nil
}

// last returns the last element of a slice or string.
func last(v interface{}) interface{} {
	switch val := v.(type) {
	case []interface{}:
		if len(val) > 0 {
			return val[len(val)-1]
		}
	case string:
		if len(val) > 0 {
			return string(val[len(val)-1])
		}
	}
	return nil
}

// keys returns the keys of a map.
func keys(v interface{}) []string {
	switch val := v.(type) {
	case map[string]interface{}:
		result := make([]string, 0, len(val))
		for k := range val {
			result = append(result, k)
		}
		return result
	default:
		return []string{}
	}
}

// values returns the values of a map.
func values(v interface{}) []interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		result := make([]interface{}, 0, len(val))
		for _, v := range val {
			result = append(result, v)
		}
		return result
	default:
		return []interface{}{}
	}
}

// rangeMap enables iteration over maps in templates.
func rangeMap(m interface{}) []map[string]interface{} {
	switch val := m.(type) {
	case map[string]interface{}:
		result := make([]map[string]interface{}, 0, len(val))
		for k, v := range val {
			result = append(result, map[string]interface{}{
				"key":   k,
				"value": v,
			})
		}
		return result
	default:
		return []map[string]interface{}{}
	}
}

// ifThenElse implements ternary operator functionality.
func ifThenElse(condition interface{}, trueVal, falseVal interface{}) interface{} {
	if isTruthy(condition) {
		return trueVal
	}
	return falseVal
}

// or implements logical OR.
func or(a, b interface{}) bool {
	return isTruthy(a) || isTruthy(b)
}

// and implements logical AND.
func and(a, b interface{}) bool {
	return isTruthy(a) && isTruthy(b)
}

// not implements logical NOT.
func not(a interface{}) bool {
	return !isTruthy(a)
}

// isTruthy determines if a value is "truthy" in template context.
func isTruthy(v interface{}) bool {
	if v == nil {
		return false
	}

	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val != ""
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0.0
	case []interface{}:
		return len(val) > 0
	case map[string]interface{}:
		return len(val) > 0
	default:
		return true
	}
}
