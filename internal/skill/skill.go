// Package skill loads SKILL.md files and exposes their content
// as additional system prompt fragments, mirroring OpenCode's Skill namespace.
//
// Layout: any SKILL.md under  .agents/skills/, .claude/skills/,
//
//	.opencode/skill/, or paths listed in config.
//
// Each SKILL.md must have YAML frontmatter with at least:
//
//	---
//	name: <skill-name>
//	description: <one-liner>
//	---
//	<markdown body injected into system prompt>
package skill

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Info describes a single loaded skill.
type Info struct {
	Name        string
	Description string
	Location    string // absolute path to SKILL.md
	Content     string // markdown body (after frontmatter)
}

// frontmatter is the YAML header parsed from a SKILL.md.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Load scans the standard skill directories relative to workDir and returns
// all valid skills found. Later entries override earlier ones on name conflict.
func Load(workDir string) ([]*Info, error) {
	searchDirs := []string{
		filepath.Join(workDir, ".agents", "skills"),
		filepath.Join(workDir, ".claude", "skills"),
		filepath.Join(workDir, ".opencode", "skill"),
		filepath.Join(workDir, ".opencode", "skills"),
	}

	seen := make(map[string]*Info)
	for _, dir := range searchDirs {
		if err := scanDir(dir, seen); err != nil {
			// directory missing is fine
			if !os.IsNotExist(err) {
				return nil, err
			}
		}
	}

	out := make([]*Info, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	return out, nil
}

// scanDir recursively walks dir looking for SKILL.md files.
func scanDir(dir string, seen map[string]*Info) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() || !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}
		info, err := parseSkill(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[skill] skipping %s: %v\n", path, err)
			return nil
		}
		seen[info.Name] = info
		return nil
	})
}

// parseSkill reads a SKILL.md and extracts frontmatter + body.
func parseSkill(path string) (*Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	// expect opening ---
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return nil, fmt.Errorf("missing YAML frontmatter")
	}

	var yamlLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		yamlLines = append(yamlLines, line)
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(yamlLines, "\n")), &fm); err != nil {
		return nil, fmt.Errorf("invalid frontmatter: %w", err)
	}
	if fm.Name == "" || fm.Description == "" {
		return nil, fmt.Errorf("frontmatter must have name and description")
	}

	// remaining lines are the body
	var bodyLines []string
	for scanner.Scan() {
		bodyLines = append(bodyLines, scanner.Text())
	}

	return &Info{
		Name:        fm.Name,
		Description: fm.Description,
		Location:    path,
		Content:     strings.Join(bodyLines, "\n"),
	}, nil
}

// SystemPrompt formats all skill contents into a block suitable for injection
// into the system prompt.
func SystemPrompt(skills []*Info) string {
	if len(skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<skills>\n")
	for _, s := range skills {
		fmt.Fprintf(&sb, "<skill name=%q>\n%s\n</skill>\n", s.Name, s.Content)
	}
	sb.WriteString("</skills>")
	return sb.String()
}
