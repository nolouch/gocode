package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillTool_ListSkills(t *testing.T) {
	// Create temporary directory with test skill
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, ".claude", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skillContent := `---
name: test-skill
description: A test skill
---
# Test Skill Content`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Execute tool without name parameter (list all skills)
	tool := &SkillTool{}
	ctx := Context{
		WorkDir: tmpDir,
		Ctx:     context.Background(),
	}
	result, err := tool.Execute(ctx, map[string]any{})

	// Verify result
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("Result is error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "test-skill") {
		t.Errorf("Output should contain skill name, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "A test skill") {
		t.Errorf("Output should contain skill description, got: %s", result.Output)
	}
}

func TestSkillTool_LoadSkill(t *testing.T) {
	// Create temporary directory with test skill
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, ".claude", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skillContent := `---
name: test-skill
description: A test skill
---
# Test Skill Content
This is the body of the test skill.`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Add a helper file
	if err := os.WriteFile(filepath.Join(skillDir, "helper.sh"), []byte("#!/bin/bash"), 0644); err != nil {
		t.Fatal(err)
	}

	// Execute tool with name parameter
	tool := &SkillTool{}
	ctx := Context{
		WorkDir: tmpDir,
		Ctx:     context.Background(),
	}
	result, err := tool.Execute(ctx, map[string]any{"name": "test-skill"})

	// Verify result
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("Result is error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "<skill_content name=\"test-skill\">") {
		t.Errorf("Output should contain skill_content tag, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Test Skill Content") {
		t.Errorf("Output should contain skill content, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "<skill_files>") {
		t.Errorf("Output should contain skill_files tag, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "helper.sh") {
		t.Errorf("Output should contain helper.sh, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "SKILL.md") {
		t.Errorf("Output should not contain SKILL.md itself, got: %s", result.Output)
	}
}

func TestSkillTool_SkillNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	tool := &SkillTool{}
	ctx := Context{
		WorkDir: tmpDir,
		Ctx:     context.Background(),
	}
	result, err := tool.Execute(ctx, map[string]any{"name": "nonexistent"})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.IsError {
		t.Error("Result should be error for nonexistent skill")
	}
	if !strings.Contains(result.Output, "not found") {
		t.Errorf("Error message should mention 'not found', got: %s", result.Output)
	}
}

func TestListSkillFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	files := []string{"file1.txt", "file2.sh", "SKILL.md", "file3.md"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create subdirectory (should be skipped)
	if err := os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	result := listSkillFiles(tmpDir, 10)

	// Verify results
	if len(result) != 3 {
		t.Errorf("Expected 3 files (excluding SKILL.md and subdir), got %d", len(result))
	}

	// Check that SKILL.md is not included
	for _, f := range result {
		if strings.Contains(f, "SKILL.md") {
			t.Error("SKILL.md should be excluded from file list")
		}
	}
}

func TestListSkillFiles_Limit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 15 test files
	for i := 0; i < 15; i++ {
		filename := filepath.Join(tmpDir, "file"+string(rune('a'+i))+".txt")
		if err := os.WriteFile(filename, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	result := listSkillFiles(tmpDir, 10)

	if len(result) != 10 {
		t.Errorf("Expected 10 files (limit), got %d", len(result))
	}
}
