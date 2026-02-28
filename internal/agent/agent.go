// Package agent defines agent configurations (build, explore, etc.),
// mirroring OpenCode's Agent.Info.
package agent

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
			"bash":        true,
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
			"bash":        true,
		},
	})

	return r
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

// List returns all registered agents.
func (r *Registry) List() []*Info {
	out := make([]*Info, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, a)
	}
	return out
}
