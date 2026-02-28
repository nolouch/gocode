package processor

import "testing"

func TestDenyToolByPolicy(t *testing.T) {
	tests := []struct {
		name   string
		agent  string
		tool   string
		args   map[string]any
		denied bool
	}{
		{name: "plan denies write", agent: "plan", tool: "write", denied: true},
		{name: "plan allows read-only bash", agent: "plan", tool: "bash", args: map[string]any{"command": "ls -la"}, denied: false},
		{name: "plan denies mutating bash", agent: "plan", tool: "bash", args: map[string]any{"command": "mkdir tmp"}, denied: true},
		{name: "build allows read bash", agent: "build", tool: "bash", args: map[string]any{"command": "ls -la"}, denied: false},
		{name: "build blocks dangerous bash", agent: "build", tool: "bash", args: map[string]any{"command": "rm -rf /"}, denied: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			denied, _ := denyToolByPolicy(tt.agent, tt.tool, tt.args)
			if denied != tt.denied {
				t.Fatalf("denyToolByPolicy(%q,%q)=%v want %v", tt.agent, tt.tool, denied, tt.denied)
			}
		})
	}
}
