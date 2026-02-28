package permission

import "testing"

func TestEvaluate_UsesLastMatchingRule(t *testing.T) {
	rules := Ruleset{
		{Permission: "*", Pattern: "*", Action: ActionDeny},
		{Permission: "read", Pattern: "*", Action: ActionAllow},
	}
	r := Evaluate("read", "README.md", rules)
	if r.Action != ActionAllow {
		t.Fatalf("expected allow, got %q", r.Action)
	}
}

func TestFromConfig(t *testing.T) {
	rules, err := FromConfig(map[string]any{
		"*":    "deny",
		"read": "allow",
		"bash": map[string]any{"*": "ask", "ls*": "allow"},
	})
	if err != nil {
		t.Fatalf("FromConfig error: %v", err)
	}
	if len(rules) != 4 {
		t.Fatalf("expected 4 rules, got %d", len(rules))
	}
}

func TestAuthorizeTool(t *testing.T) {
	rules := Ruleset{
		{Permission: "*", Pattern: "*", Action: ActionDeny},
		{Permission: "bash", Pattern: "*", Action: ActionAllow},
	}
	if denied, _ := AuthorizeTool("plan", "bash", map[string]any{"command": "ls -la"}, rules); denied {
		t.Fatal("expected read-only bash to be allowed")
	}
	if denied, _ := AuthorizeTool("plan", "bash", map[string]any{"command": "mkdir foo"}, rules); !denied {
		t.Fatal("expected mutating bash to be denied")
	}
}
