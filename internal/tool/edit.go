package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EditTool performs a precise string replacement in a file.
// Fails if old_string is not found or matches more than once.
type EditTool struct{}

func (t *EditTool) ID() string { return "edit" }
func (t *EditTool) Description() string {
	return "Edit a file by replacing an exact string. Fails if old_string is not found or is not unique. Prefer this over write_file for modifying existing files."
}
func (t *EditTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":       map[string]any{"type": "string", "description": "Path to the file"},
			"old_string": map[string]any{"type": "string", "description": "Exact string to find (must appear exactly once)"},
			"new_string": map[string]any{"type": "string", "description": "Replacement string"},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (t *EditTool) Execute(ctx Context, args map[string]any) (Result, error) {
	p, _ := args["path"].(string)
	oldStr, _ := args["old_string"].(string)
	newStr, _ := args["new_string"].(string)

	if p == "" || oldStr == "" {
		return Result{IsError: true, Output: "edit requires 'path' and 'old_string'"}, nil
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(ctx.WorkDir, p)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}
	content := string(data)

	count := strings.Count(content, oldStr)
	if count == 0 {
		return Result{IsError: true, Output: fmt.Sprintf("old_string not found in %s", p)}, nil
	}
	if count > 1 {
		return Result{IsError: true, Output: fmt.Sprintf("old_string matches %d times in %s — must be unique", count, p)}, nil
	}

	updated := strings.Replace(content, oldStr, newStr, 1)
	if err := os.WriteFile(p, []byte(updated), 0o644); err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}
	return Result{Output: fmt.Sprintf("Edited %s", p), Title: p}, nil
}
