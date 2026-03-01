package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nolouch/gocode/internal/skill"
)

// SkillTool loads skills on demand, allowing AI to access specialized
// instructions and workflows without loading everything at startup.
type SkillTool struct{}

func (t *SkillTool) ID() string { return "skill" }

func (t *SkillTool) Description() string {
	return "Load a skill to access specialized instructions and workflows.\n\n" +
		"Use this tool when you need specific guidance for tasks like:\n" +
		"- Creating git releases and changelogs\n" +
		"- Database migrations\n" +
		"- Code review workflows\n" +
		"- And other specialized tasks\n\n" +
		"Call without 'name' parameter to list all available skills.\n" +
		"Call with 'name' parameter to load a specific skill."
}

func (t *SkillTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The name of the skill to load (omit to list all available skills)",
			},
		},
	}
}

func (t *SkillTool) Execute(ctx Context, args map[string]any) (Result, error) {
	name, _ := args["name"].(string)

	// Load all skills
	skills, err := skill.Load(ctx.WorkDir)
	if err != nil {
		return Result{IsError: true, Output: fmt.Sprintf("Failed to load skills: %v", err)}, nil
	}

	// If no name provided, list all available skills
	if name == "" {
		if len(skills) == 0 {
			return Result{Output: "No skills available."}, nil
		}

		var sb strings.Builder
		sb.WriteString("Available skills:\n\n")
		for _, s := range skills {
			fmt.Fprintf(&sb, "- **%s**: %s\n  Location: %s\n\n", s.Name, s.Description, s.Location)
		}
		return Result{Output: sb.String(), Title: "Available Skills"}, nil
	}

	// Find the requested skill
	var target *skill.Info
	for _, s := range skills {
		if s.Name == name {
			target = s
			break
		}
	}

	if target == nil {
		return Result{IsError: true, Output: fmt.Sprintf("Skill %q not found", name)}, nil
	}

	// Scan skill directory for additional files
	skillDir := filepath.Dir(target.Location)
	files := listSkillFiles(skillDir, 10)

	// Format output
	var output strings.Builder
	fmt.Fprintf(&output, "<skill_content name=%q>\n", target.Name)
	fmt.Fprintf(&output, "# Skill: %s\n\n", target.Name)
	output.WriteString(target.Content)
	output.WriteString("\n\n")
	fmt.Fprintf(&output, "Base directory: file://%s\n", skillDir)
	output.WriteString("Relative paths are relative to this base directory.\n\n")

	if len(files) > 0 {
		output.WriteString("<skill_files>\n")
		for _, f := range files {
			fmt.Fprintf(&output, "<file>%s</file>\n", f)
		}
		output.WriteString("</skill_files>\n")
	}
	output.WriteString("</skill_content>")

	return Result{
		Output: output.String(),
		Title:  fmt.Sprintf("Skill: %s", target.Name),
	}, nil
}

// listSkillFiles scans the skill directory for additional files,
// returning at most limit files. Skips SKILL.md itself.
func listSkillFiles(dir string, limit int) []string {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return files
	}

	for _, e := range entries {
		if len(files) >= limit {
			break
		}
		if e.IsDir() {
			continue
		}
		// Skip SKILL.md itself
		if strings.EqualFold(e.Name(), "SKILL.md") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	return files
}
