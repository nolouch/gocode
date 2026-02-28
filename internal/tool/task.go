package tool

import (
	"fmt"
	"strings"
)

type TaskRequest struct {
	Description string
	Prompt      string
	Subagent    string
	TaskID      string
	Command     string
}

type TaskResult struct {
	TaskID     string
	Output     string
	Subagent   string
	DurationMs int64
}

// TaskTool launches a subagent task and returns the subagent response.
type TaskTool struct{}

func (t *TaskTool) ID() string { return "task" }
func (t *TaskTool) Description() string {
	return "Launch a subagent to handle a focused task and return its output."
}
func (t *TaskTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"description":   map[string]any{"type": "string", "description": "A short (3-5 words) description of the task"},
			"prompt":        map[string]any{"type": "string", "description": "The task for the agent to perform"},
			"subagent_type": map[string]any{"type": "string", "description": "The type of specialized agent to use for this task"},
			"task_id":       map[string]any{"type": "string", "description": "Optional task/session ID to resume"},
			"command":       map[string]any{"type": "string", "description": "Command that triggered this task"},
		},
		"required": []string{"description", "prompt", "subagent_type"},
	}
}

func (t *TaskTool) Execute(ctx Context, args map[string]any) (Result, error) {
	if ctx.RunTask == nil {
		return Result{IsError: true, Output: "task runner unavailable in this context"}, nil
	}
	description, _ := args["description"].(string)
	prompt, _ := args["prompt"].(string)
	subagent, _ := args["subagent_type"].(string)
	taskID, _ := args["task_id"].(string)
	command, _ := args["command"].(string)

	if strings.TrimSpace(description) == "" || strings.TrimSpace(prompt) == "" || strings.TrimSpace(subagent) == "" {
		return Result{IsError: true, Output: "task requires description, prompt, and subagent_type"}, nil
	}

	out, err := ctx.RunTask(TaskRequest{
		Description: description,
		Prompt:      prompt,
		Subagent:    subagent,
		TaskID:      taskID,
		Command:     command,
	})
	if err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}

	payload := fmt.Sprintf("task_id: %s\n\n<task_result>\n%s\n</task_result>", out.TaskID, strings.TrimSpace(out.Output))
	metaSubagent := out.Subagent
	if metaSubagent == "" {
		metaSubagent = subagent
	}
	return Result{
		Output: payload,
		Title:  description,
		Metadata: map[string]any{
			"task_id":     out.TaskID,
			"subagent":    metaSubagent,
			"duration_ms": out.DurationMs,
		},
	}, nil
}
