package tool

import (
	"os"
	"path/filepath"
)

const todoFile = ".opengocode/todo.md"

// TodoReadTool reads the current todo list.
type TodoReadTool struct{}

func (t *TodoReadTool) ID() string { return "todo_read" }
func (t *TodoReadTool) Description() string {
	return "Read the current task/todo list from .opengocode/todo.md."
}
func (t *TodoReadTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *TodoReadTool) Execute(ctx Context, args map[string]any) (Result, error) {
	p := filepath.Join(ctx.WorkDir, todoFile)
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Result{Output: "(no todo list yet)"}, nil
	}
	if err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}
	return Result{Output: string(data), Title: todoFile}, nil
}

// TodoWriteTool overwrites the todo list.
type TodoWriteTool struct{}

func (t *TodoWriteTool) ID() string { return "todo_write" }
func (t *TodoWriteTool) Description() string {
	return "Overwrite the task/todo list in .opengocode/todo.md. Use markdown checkboxes: '- [ ] task' and '- [x] done'."
}
func (t *TodoWriteTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{"type": "string", "description": "Full markdown content for the todo list"},
		},
		"required": []string{"content"},
	}
}
func (t *TodoWriteTool) Execute(ctx Context, args map[string]any) (Result, error) {
	content, _ := args["content"].(string)
	if content == "" {
		return Result{IsError: true, Output: "todo_write requires 'content'"}, nil
	}
	p := filepath.Join(ctx.WorkDir, todoFile)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}
	return Result{Output: "Todo list updated.", Title: todoFile}, nil
}
