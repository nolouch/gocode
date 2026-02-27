// Package tool defines the Tool interface and a registry of built-in tools.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Context carries per-call metadata for a tool execution.
type Context struct {
	SessionID string
	MessageID string
	CallID    string
	Agent     string
	WorkDir   string
	Ctx       context.Context
}

// Result is what a tool returns.
type Result struct {
	Output   string
	Title    string
	Metadata map[string]any
	IsError  bool
}

// Tool is the interface all tools implement.
type Tool interface {
	ID() string
	Description() string
	// Schema returns a JSON Schema describing the input parameters.
	Schema() map[string]any
	// Execute runs the tool. args is the decoded JSON input.
	Execute(ctx Context, args map[string]any) (Result, error)
}

// ─────────────────────────────────────────────
// Registry
// ─────────────────────────────────────────────

// Registry holds all registered tools by their ID.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a new Registry pre-populated with built-in tools.
func NewRegistry() *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	r.Register(&ReadFileTool{})
	r.Register(&WriteFileTool{})
	r.Register(&ListDirTool{})
	r.Register(&BashTool{})
	return r
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.ID()] = t
}

// Get returns a tool by ID.
func (r *Registry) Get(id string) (Tool, bool) {
	t, ok := r.tools[id]
	return t, ok
}

// All returns all registered tools.
func (r *Registry) All() map[string]Tool {
	out := make(map[string]Tool, len(r.tools))
	for k, v := range r.tools {
		out[k] = v
	}
	return out
}

// ─────────────────────────────────────────────
// Built-in tools
// ─────────────────────────────────────────────

// ReadFileTool reads a file from the filesystem.
type ReadFileTool struct{}

func (t *ReadFileTool) ID() string          { return "read_file" }
func (t *ReadFileTool) Description() string { return "Read the contents of a file." }
func (t *ReadFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Absolute or relative path to the file"},
		},
		"required": []string{"path"},
	}
}
func (t *ReadFileTool) Execute(ctx Context, args map[string]any) (Result, error) {
	p, _ := args["path"].(string)
	if !filepath.IsAbs(p) {
		p = filepath.Join(ctx.WorkDir, p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}
	return Result{Output: string(data), Title: p}, nil
}

// WriteFileTool writes content to a file, creating directories as needed.
type WriteFileTool struct{}

func (t *WriteFileTool) ID() string          { return "write_file" }
func (t *WriteFileTool) Description() string { return "Write content to a file (overwrite)." }
func (t *WriteFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Path to the file"},
			"content": map[string]any{"type": "string", "description": "Content to write"},
		},
		"required": []string{"path", "content"},
	}
}
func (t *WriteFileTool) Execute(ctx Context, args map[string]any) (Result, error) {
	p, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if !filepath.IsAbs(p) {
		p = filepath.Join(ctx.WorkDir, p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}
	return Result{Output: fmt.Sprintf("Written %d bytes to %s", len(content), p), Title: p}, nil
}

// ListDirTool lists directory contents.
type ListDirTool struct{}

func (t *ListDirTool) ID() string          { return "list_dir" }
func (t *ListDirTool) Description() string { return "List files and directories in a path." }
func (t *ListDirTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Directory path"},
		},
		"required": []string{"path"},
	}
}
func (t *ListDirTool) Execute(ctx Context, args map[string]any) (Result, error) {
	p, _ := args["path"].(string)
	if !filepath.IsAbs(p) {
		p = filepath.Join(ctx.WorkDir, p)
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}
	type entry struct {
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size,omitempty"`
	}
	var out []entry
	for _, e := range entries {
		info, _ := e.Info()
		sz := int64(0)
		if info != nil && !e.IsDir() {
			sz = info.Size()
		}
		out = append(out, entry{Name: e.Name(), IsDir: e.IsDir(), Size: sz})
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return Result{Output: string(b), Title: p}, nil
}

// BashTool runs a shell command and returns stdout+stderr.
type BashTool struct{}

func (t *BashTool) ID() string          { return "bash" }
func (t *BashTool) Description() string { return "Run a bash command and return its output." }
func (t *BashTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":    map[string]any{"type": "string", "description": "Shell command to execute"},
			"timeout_ms": map[string]any{"type": "integer", "description": "Timeout in milliseconds (default 30000)"},
		},
		"required": []string{"command"},
	}
}
func (t *BashTool) Execute(ctx Context, args map[string]any) (Result, error) {
	cmd, _ := args["command"].(string)

	c := exec.CommandContext(ctx.Ctx, "bash", "-c", cmd)
	c.Dir = ctx.WorkDir
	out, err := c.CombinedOutput()
	output := string(out)
	if err != nil {
		return Result{
			IsError: true,
			Output:  fmt.Sprintf("Exit error: %v\n%s", err, output),
			Title:   cmd,
		}, nil
	}
	return Result{Output: output, Title: cmd}, nil
}
