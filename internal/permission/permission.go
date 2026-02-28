package permission

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
	ActionAsk   Action = "ask"
)

type Rule struct {
	Permission string
	Pattern    string
	Action     Action
}

type Ruleset []Rule

func Merge(sets ...Ruleset) Ruleset {
	var out Ruleset
	for _, s := range sets {
		out = append(out, s...)
	}
	return out
}

func Evaluate(permission, pattern string, sets ...Ruleset) Rule {
	merged := Merge(sets...)
	for i := len(merged) - 1; i >= 0; i-- {
		r := merged[i]
		if wildcardMatch(permission, r.Permission) && wildcardMatch(pattern, r.Pattern) {
			return r
		}
	}
	return Rule{Permission: permission, Pattern: "*", Action: ActionAsk}
}

func ToolAllowed(toolID string, rules Ruleset) bool {
	if len(rules) == 0 {
		return true
	}
	r := Evaluate(toolID, "*", rules)
	return r.Action == ActionAllow
}

func FromConfig(raw map[string]any) (Ruleset, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(Ruleset, 0, len(raw))
	for permission, value := range raw {
		switch v := value.(type) {
		case string:
			a, err := parseAction(v)
			if err != nil {
				return nil, fmt.Errorf("permission %q: %w", permission, err)
			}
			out = append(out, Rule{Permission: permission, Pattern: "*", Action: a})
		case map[string]any:
			for pattern, actionRaw := range v {
				a, err := parseAction(fmt.Sprint(actionRaw))
				if err != nil {
					return nil, fmt.Errorf("permission %q pattern %q: %w", permission, pattern, err)
				}
				out = append(out, Rule{Permission: permission, Pattern: pattern, Action: a})
			}
		case map[any]any:
			for patternRaw, actionRaw := range v {
				pattern := fmt.Sprint(patternRaw)
				a, err := parseAction(fmt.Sprint(actionRaw))
				if err != nil {
					return nil, fmt.Errorf("permission %q pattern %q: %w", permission, pattern, err)
				}
				out = append(out, Rule{Permission: permission, Pattern: pattern, Action: a})
			}
		default:
			return nil, fmt.Errorf("permission %q has unsupported value type %T", permission, value)
		}
	}
	return out, nil
}

func parseAction(v string) (Action, error) {
	s := strings.ToLower(strings.TrimSpace(v))
	switch Action(s) {
	case ActionAllow, ActionDeny, ActionAsk:
		return Action(s), nil
	default:
		return "", fmt.Errorf("unsupported action %q", v)
	}
}

func wildcardMatch(value, pattern string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	ok, err := filepath.Match(pattern, value)
	if err == nil {
		return ok
	}
	return value == pattern
}
