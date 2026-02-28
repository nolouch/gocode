package tool

import (
	"strings"
	"testing"
)

func TestValidateArgs(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"age":  map[string]any{"type": "integer"},
		},
		"required": []string{"name"},
	}

	if err := ValidateArgs(schema, map[string]any{"name": "alice", "age": float64(30)}); err != nil {
		t.Fatalf("expected valid args, got %v", err)
	}
	if err := ValidateArgs(schema, map[string]any{"age": float64(30)}); err == nil {
		t.Fatal("expected missing required argument error")
	}
	if err := ValidateArgs(schema, map[string]any{"name": "alice", "age": "30"}); err == nil {
		t.Fatal("expected invalid type error")
	}
}

func TestNormalizeResult(t *testing.T) {
	res := NormalizeResult(Result{Output: strings.Repeat("a", MaxOutputBytes+1024)})
	if res.Metadata == nil {
		t.Fatal("metadata should be initialized")
	}
	if truncated, _ := res.Metadata["truncated"].(bool); !truncated {
		t.Fatal("expected truncated=true")
	}
	if !strings.Contains(res.Output, "[output truncated") {
		t.Fatal("expected truncation suffix in output")
	}
}
