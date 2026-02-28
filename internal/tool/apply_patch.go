package tool

import (
	"fmt"
	"os/exec"
	"strings"
)

// ApplyPatchTool applies a unified diff patch to files in the current repository.
type ApplyPatchTool struct{}

func (t *ApplyPatchTool) ID() string { return "apply_patch" }
func (t *ApplyPatchTool) Description() string {
	return "Apply a unified diff patch in the current repository."
}
func (t *ApplyPatchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"patch": map[string]any{
				"type":        "string",
				"description": "Unified diff patch content",
			},
		},
		"required": []string{"patch"},
	}
}

func (t *ApplyPatchTool) Execute(ctx Context, args map[string]any) (Result, error) {
	patch, _ := args["patch"].(string)
	if strings.TrimSpace(patch) == "" {
		return Result{IsError: true, Output: "apply_patch requires a non-empty 'patch' string"}, nil
	}

	cmd := exec.CommandContext(ctx.Ctx, "git", "apply", "--whitespace=nowarn", "-")
	cmd.Dir = ctx.WorkDir
	cmd.Stdin = strings.NewReader(patch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{IsError: true, Output: fmt.Sprintf("git apply failed: %v\n%s", err, string(out))}, nil
	}
	return Result{Output: "Patch applied successfully.", Title: "apply_patch"}, nil
}
