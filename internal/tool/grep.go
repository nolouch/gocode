package tool

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GrepTool searches file contents using ripgrep (falls back to grep).
type GrepTool struct{}

func (t *GrepTool) ID() string { return "grep" }
func (t *GrepTool) Description() string {
	return "Search for a pattern in files. Returns matching lines with file path and line number."
}
func (t *GrepTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern":        map[string]any{"type": "string", "description": "Regex or literal pattern to search for"},
			"path":           map[string]any{"type": "string", "description": "Directory or file to search (defaults to working directory)"},
			"glob":           map[string]any{"type": "string", "description": "File glob filter, e.g. '*.go'"},
			"case_sensitive": map[string]any{"type": "boolean", "description": "Case sensitive search (default true)"},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepTool) Execute(ctx Context, args map[string]any) (Result, error) {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return Result{IsError: true, Output: "grep requires a 'pattern'"}, nil
	}

	searchPath, _ := args["path"].(string)
	if searchPath == "" {
		searchPath = ctx.WorkDir
	} else if !filepath.IsAbs(searchPath) {
		searchPath = filepath.Join(ctx.WorkDir, searchPath)
	}

	glob, _ := args["glob"].(string)
	caseSensitive, _ := args["case_sensitive"].(bool)

	// Try ripgrep first, fall back to grep
	if out, err := runRipgrep(pattern, searchPath, glob, caseSensitive); err == nil {
		return Result{Output: truncate(out, 50000), Title: pattern}, nil
	}
	out, err := runGrep(pattern, searchPath, glob, caseSensitive)
	if err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}
	return Result{Output: truncate(out, 50000), Title: pattern}, nil
}

func runRipgrep(pattern, path, glob string, caseSensitive bool) (string, error) {
	args := []string{"--line-number", "--no-heading", "--color=never"}
	if !caseSensitive {
		args = append(args, "-i")
	}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	args = append(args, pattern, path)

	cmd := exec.Command("rg", args...)
	out, err := cmd.Output()
	if err != nil {
		// exit code 1 means no matches — not an error
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return "", nil
		}
		return "", err
	}
	return string(out), nil
}

func runGrep(pattern, path, glob string, caseSensitive bool) (string, error) {
	args := []string{"-rn", "--include"}
	if glob != "" {
		args = append(args, glob)
	} else {
		args = append(args, "*")
	}
	if !caseSensitive {
		args = append(args, "-i")
	}
	args = append(args, pattern, path)

	cmd := exec.Command("grep", args...)
	out, err := cmd.Output()
	if err != nil {
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return "", nil
		}
		return "", fmt.Errorf("grep: %w", err)
	}
	return string(out), nil
}

// truncate limits output to maxBytes, appending a notice if trimmed.
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// trim to last newline within limit
	trimmed := s[:maxBytes]
	if idx := strings.LastIndex(trimmed, "\n"); idx > 0 {
		trimmed = trimmed[:idx]
	}
	return trimmed + fmt.Sprintf("\n\n[output truncated — %d bytes omitted]", len(s)-len(trimmed))
}
