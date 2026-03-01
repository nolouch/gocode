package systemprompt

import (
	"strings"
	"testing"

	"github.com/nolouch/gocode/internal/agent"
)

func TestBuilder_Build(t *testing.T) {
	builder := New("/home/user/project", true)
	ag := &agent.Info{
		Name:        "build",
		Description: "Default coding agent",
		Prompt:      "Additional agent-specific instructions.",
	}
	extras := []string{"Extra skill prompt"}

	result := builder.Build(ag, extras)

	if len(result) != 3 {
		t.Errorf("expected 3 parts, got %d", len(result))
	}

	// Check base prompt is included
	if !strings.Contains(result[0], "You are gcode") {
		t.Error("base prompt should contain 'You are gcode'")
	}

	// Check environment context
	if !strings.Contains(result[0], "/home/user/project") {
		t.Error("should contain working directory")
	}
	if !strings.Contains(result[0], "Git repository: yes") {
		t.Error("should indicate git repository")
	}

	// Check agent prompt
	if result[1] != "Additional agent-specific instructions." {
		t.Errorf("agent prompt mismatch: %s", result[1])
	}

	// Check extras
	if result[2] != "Extra skill prompt" {
		t.Errorf("extra prompt mismatch: %s", result[2])
	}
}

func TestBuilder_BuildEnvironmentContext(t *testing.T) {
	builder := New("/tmp/test", false)
	ctx := builder.buildEnvironmentContext()

	if !strings.Contains(ctx, "Working directory: /tmp/test") {
		t.Error("should contain working directory")
	}
	if !strings.Contains(ctx, "Git repository: no") {
		t.Error("should indicate no git repository")
	}
	if !strings.Contains(ctx, "Platform:") {
		t.Error("should contain platform")
	}
	if !strings.Contains(ctx, "Current date:") {
		t.Error("should contain current date")
	}
}
