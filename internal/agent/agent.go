// Package agent defines agent configurations (build, explore, etc.),
// mirroring OpenCode's Agent.Info.
package agent

import (
	"fmt"
	"strings"

	"github.com/nolouch/gocode/internal/permission"
)

// Mode defines where an agent can be used.
type Mode string

const (
	ModePrimary  Mode = "primary"
	ModeSubagent Mode = "subagent"
	ModeAll      Mode = "all"
)

// Info describes a named agent configuration.
type Info struct {
	Name        string
	Description string
	Mode        Mode
	// System prompt override (appended after base system prompt)
	Prompt string
	// Model override (empty = use session default)
	ProviderID string
	ModelID    string
	// MaxSteps limits the agent loop iterations (0 = unlimited)
	MaxSteps int
	// Tools explicitly denied for this agent (tool ID -> true)
	DeniedTools map[string]bool
	// Temperature override (0 = use provider default)
	Temperature float64
	// Permission ruleset (allow/deny/ask) for tool access
	Permissions permission.Ruleset
}

// Override applies config-driven changes to a built-in or custom agent.
type Override struct {
	Disable     bool
	Name        string
	Description string
	Mode        string
	Prompt      string
	ProviderID  string
	ModelID     string
	Steps       int
	Temperature float64
	DeniedTools []string
	Permission  map[string]any
}

// Registry holds all named agents.
type Registry struct {
	agents map[string]*Info
}

// NewRegistry returns a registry pre-populated with built-in agents.
func NewRegistry() *Registry {
	r := &Registry{agents: make(map[string]*Info)}

	// "build" – the default primary coding agent
	r.Register(&Info{
		Name:        "build",
		Description: "Default coding agent. Reads, writes, and runs code.",
		Mode:        ModePrimary,
		Permissions: permission.Ruleset{{Permission: "*", Pattern: "*", Action: permission.ActionAllow}},
	})

	// "explore" – read-only, fast codebase exploration
	r.Register(&Info{
		Name:        "explore",
		Description: "Read-only agent for fast codebase exploration.",
		Mode:        ModeSubagent,
		Prompt:      "You are a fast, read-only codebase explorer. Do not write or modify files.",
		DeniedTools: map[string]bool{
			"write":       true,
			"write_file":  true,
			"edit":        true,
			"apply_patch": true,
		},
		Permissions: permission.Ruleset{
			{Permission: "*", Pattern: "*", Action: permission.ActionDeny},
			{Permission: "read", Pattern: "*", Action: permission.ActionAllow},
			{Permission: "list", Pattern: "*", Action: permission.ActionAllow},
			{Permission: "glob", Pattern: "*", Action: permission.ActionAllow},
			{Permission: "grep", Pattern: "*", Action: permission.ActionAllow},
			{Permission: "web_fetch", Pattern: "*", Action: permission.ActionAllow},
			{Permission: "bash", Pattern: "*", Action: permission.ActionAllow},
		},
	})

	// "plan" – strict planning mode, no mutating tools.
	r.Register(&Info{
		Name:        "plan",
		Description: "Read-only planning agent. Analyze and propose changes without modifying files.",
		Mode:        ModePrimary,
		Prompt:      "You are in plan mode. You must not modify files or run mutating shell commands.",
		DeniedTools: map[string]bool{
			"write":       true,
			"write_file":  true,
			"edit":        true,
			"apply_patch": true,
		},
		Permissions: permission.Ruleset{
			{Permission: "*", Pattern: "*", Action: permission.ActionDeny},
			{Permission: "read", Pattern: "*", Action: permission.ActionAllow},
			{Permission: "list", Pattern: "*", Action: permission.ActionAllow},
			{Permission: "glob", Pattern: "*", Action: permission.ActionAllow},
			{Permission: "grep", Pattern: "*", Action: permission.ActionAllow},
			{Permission: "web_fetch", Pattern: "*", Action: permission.ActionAllow},
			{Permission: "bash", Pattern: "*", Action: permission.ActionAllow},
		},
	})

	return r
}

// ApplyOverrides mutates registry entries based on config values.
func (r *Registry) ApplyOverrides(overrides map[string]Override) error {
	for key, v := range overrides {
		if v.Disable {
			delete(r.agents, key)
			continue
		}

		a := r.agents[key]
		if a == nil {
			a = &Info{
				Name:        key,
				Mode:        ModeAll,
				DeniedTools: map[string]bool{},
				Permissions: permission.Ruleset{{Permission: "*", Pattern: "*", Action: permission.ActionAllow}},
			}
		}

		if v.Name != "" {
			a.Name = v.Name
		}
		if v.Description != "" {
			a.Description = v.Description
		}
		if v.Mode != "" {
			m, err := parseMode(v.Mode)
			if err != nil {
				return err
			}
			a.Mode = m
		}
		if v.Prompt != "" {
			a.Prompt = v.Prompt
		}
		if v.ProviderID != "" {
			a.ProviderID = v.ProviderID
		}
		if v.ModelID != "" {
			a.ModelID = v.ModelID
		}
		if v.Steps > 0 {
			a.MaxSteps = v.Steps
		}
		if v.Temperature > 0 {
			a.Temperature = v.Temperature
		}
		if len(v.DeniedTools) > 0 {
			if a.DeniedTools == nil {
				a.DeniedTools = map[string]bool{}
			}
			for _, id := range v.DeniedTools {
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				a.DeniedTools[id] = true
			}
		}
		if len(v.Permission) > 0 {
			rules, err := permission.FromConfig(v.Permission)
			if err != nil {
				return err
			}
			a.Permissions = rules
		}

		r.agents[key] = a
	}
	return nil
}

func parseMode(raw string) (Mode, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	switch Mode(s) {
	case ModePrimary, ModeSubagent, ModeAll:
		return Mode(s), nil
	default:
		return "", fmt.Errorf("invalid agent mode: %s", raw)
	}
}

// Register adds or replaces an agent.
func (r *Registry) Register(a *Info) {
	r.agents[a.Name] = a
}

// Get returns an agent by name (falls back to "build").
func (r *Registry) Get(name string) *Info {
	if a, ok := r.agents[name]; ok {
		return a
	}
	return r.agents["build"]
}

// Lookup returns an agent by name without fallback.
func (r *Registry) Lookup(name string) (*Info, bool) {
	a, ok := r.agents[name]
	return a, ok
}

// List returns all registered agents.
func (r *Registry) List() []*Info {
	out := make([]*Info, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, a)
	}
	return out
}
