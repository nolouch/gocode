package tool

import (
	"fmt"
	"math"
	"strings"
)

const MaxOutputBytes = 50 * 1024

// ValidateArgs performs lightweight JSON-schema validation for common tool schemas.
func ValidateArgs(schema map[string]any, args map[string]any) error {
	if schema == nil {
		return nil
	}
	typeName, _ := schema["type"].(string)
	if typeName != "" && typeName != "object" {
		return nil
	}

	missing := requiredFields(schema)
	for _, field := range missing {
		if _, ok := args[field]; !ok {
			return fmt.Errorf("missing required argument: %s", field)
		}
	}

	props, _ := schema["properties"].(map[string]any)
	for key, value := range args {
		defRaw, ok := props[key]
		if !ok {
			continue
		}
		def, ok := defRaw.(map[string]any)
		if !ok {
			continue
		}
		expected, _ := def["type"].(string)
		if expected == "" {
			continue
		}
		if !typeMatches(expected, value) {
			return fmt.Errorf("invalid type for %s: expected %s", key, expected)
		}
	}

	return nil
}

// NormalizeResult applies common output shaping for all tool results.
func NormalizeResult(res Result) Result {
	if res.Metadata == nil {
		res.Metadata = map[string]any{}
	}
	originalBytes := len(res.Output)
	res.Metadata["output_bytes"] = originalBytes

	if originalBytes > MaxOutputBytes {
		trimmed := res.Output[:MaxOutputBytes]
		if idx := strings.LastIndex(trimmed, "\n"); idx > 0 {
			trimmed = trimmed[:idx]
		}
		res.Output = trimmed + fmt.Sprintf("\n\n[output truncated — %d bytes omitted]", originalBytes-len(trimmed))
		res.Metadata["truncated"] = true
		return res
	}

	res.Metadata["truncated"] = false
	return res
}

func requiredFields(schema map[string]any) []string {
	requiredRaw, ok := schema["required"]
	if !ok {
		return nil
	}
	switch r := requiredRaw.(type) {
	case []string:
		return r
	case []any:
		out := make([]string, 0, len(r))
		for _, item := range r {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func typeMatches(expected string, value any) bool {
	switch expected {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		switch n := value.(type) {
		case int, int32, int64, uint, uint32, uint64:
			return true
		case float64:
			return math.Trunc(n) == n
		default:
			return false
		}
	case "number":
		switch value.(type) {
		case int, int32, int64, uint, uint32, uint64, float64, float32:
			return true
		default:
			return false
		}
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	default:
		return true
	}
}
