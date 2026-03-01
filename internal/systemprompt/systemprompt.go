// Package systemprompt builds system prompts for the LLM, including
// base instructions, agent-specific overrides, and environment context.
package systemprompt

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/nolouch/gcode/internal/agent"
)

//go:embed prompts/base.txt
var basePrompt string

// Builder constructs system prompts with environment context.
type Builder struct {
	WorkDir  string
	Platform string
	IsGitRepo bool
}

// Build assembles the complete system prompt for an agent.
// It combines:
//  1. Base prompt (from prompts/base.txt)
//  2. Agent-specific prompt override (if any)
//  3. Environment context (working directory, platform, date)
//  4. Extra prompt fragments (e.g., from skills)
func (b *Builder) Build(ag *agent.Info, extras []string) []string {
	var parts []string

	// 1. Base prompt with environment context
	envContext := b.buildEnvironmentContext()
	baseWithEnv := basePrompt + "\n\n" + envContext
	parts = append(parts, baseWithEnv)

	// 2. Agent-specific prompt override
	if ag.Prompt != "" {
		parts = append(parts, ag.Prompt)
	}

	// 3. Extra fragments (skills, etc.)
	parts = append(parts, extras...)

	return parts
}

// buildEnvironmentContext generates the environment section of the system prompt.
func (b *Builder) buildEnvironmentContext() string {
	var sb strings.Builder

	sb.WriteString("## Environment\n")
	sb.WriteString(fmt.Sprintf("- Working directory: %s\n", b.WorkDir))
	sb.WriteString(fmt.Sprintf("- Platform: %s\n", b.Platform))

	if b.IsGitRepo {
		sb.WriteString("- Git repository: yes\n")
	} else {
		sb.WriteString("- Git repository: no\n")
	}

	// Current date
	now := time.Now()
	sb.WriteString(fmt.Sprintf("- Current date: %s\n", now.Format("2006-01-02")))

	return sb.String()
}

// New creates a Builder with the given working directory.
// It auto-detects platform and git repository status.
func New(workDir string, isGitRepo bool) *Builder {
	return &Builder{
		WorkDir:   workDir,
		Platform:  runtime.GOOS,
		IsGitRepo: isGitRepo,
	}
}
