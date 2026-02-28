package tool

import (
	"context"
	"strings"
	"testing"
)

func TestTaskToolExecute(t *testing.T) {
	task := &TaskTool{}
	called := false
	res, err := task.Execute(Context{
		Ctx: context.Background(),
		RunTask: func(req TaskRequest) (TaskResult, error) {
			called = true
			if req.Subagent != "explore" {
				t.Fatalf("unexpected subagent: %s", req.Subagent)
			}
			return TaskResult{TaskID: "sess-123", Output: "done"}, nil
		},
	}, map[string]any{
		"description":   "Analyze code",
		"prompt":        "Find API handlers",
		"subagent_type": "explore",
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if !called {
		t.Fatal("expected RunTask callback to be called")
	}
	if res.IsError {
		t.Fatalf("unexpected tool error result: %s", res.Output)
	}
	if !strings.Contains(res.Output, "task_id: sess-123") {
		t.Fatalf("missing task_id output, got: %s", res.Output)
	}
}

func TestTaskToolExecuteMissingRunner(t *testing.T) {
	task := &TaskTool{}
	res, err := task.Execute(Context{}, map[string]any{
		"description":   "Analyze",
		"prompt":        "Check code",
		"subagent_type": "explore",
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result when RunTask is unavailable")
	}
}
