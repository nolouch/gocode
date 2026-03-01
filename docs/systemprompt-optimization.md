# System Prompt Optimization

## Summary

This document describes the system prompt optimization work for gcode, inspired by the opencode project's approach.

## What Was Done

### 1. Created Standalone System Prompt Module

**New files**:
- `internal/systemprompt/systemprompt.go` - System prompt builder
- `internal/systemprompt/prompts/base.txt` - Base prompt text
- `internal/systemprompt/systemprompt_test.go` - Unit tests

### 2. Learned and Adapted from opencode

**Adopted best practices from opencode**:
- Detailed editing constraints (ASCII preference, comment guidelines, apply_patch priority)
- Tool usage strategies (specialized tools over shell, parallel execution)
- Git and workspace hygiene rules (no casual reverts, no destructive commands)
- Output formatting guidelines (concise and friendly, file reference format)

**Customized for Go development**:
- Removed frontend-related guidance (not applicable to gcode)
- Added Go-specific best practices:
  - Use gofmt/goimports for formatting
  - Use go mod tidy for dependency management
  - Table-driven tests
  - context.Context for cancellation and timeouts
  - Explicit error handling
  - Meaningful package names

### 3. Environment Information Injection

System prompt now includes:
- Working directory path
- Platform information (darwin/linux/windows)
- Git repository status (yes/no)
- Current date

### 4. Updated loop.go Integration

**Changes made**:
- Added `systemprompt` package import
- Simplified `buildSystemPrompt` function to delegate to new module
- Added `isGitRepository` helper function to detect git repos
- Added `os` package import for filesystem checks

## Architecture Improvement

### Before
```go
// Hard-coded in loop.go
base := fmt.Sprintf(`You are gcode, an expert AI coding assistant.
Current working directory: %s
...`, workDir)
```

### After
```go
// Modular, testable, extensible
builder := systemprompt.New(workDir, isGitRepo)
return builder.Build(ag, extras)
```

## Benefits

1. **Separation of concerns**: System prompt logic extracted from loop.go
2. **Testability**: Independent unit tests verify prompt generation
3. **Maintainability**: Prompt text in separate .txt files, easy to edit
4. **Extensibility**: Easy to add model-specific optimizations in the future
5. **Environment awareness**: LLM now knows working directory, platform, git status, etc.

## Test Results

All tests passing:
- `go test ./internal/loop/...` ✓
- `go test ./internal/systemprompt/...` ✓
- `go build ./...` ✓

## Future Optional Enhancements (P2 Priority)

If multi-provider support is needed, can add:
- `prompts/anthropic.txt` - Claude-optimized version
- `prompts/openai.txt` - GPT-optimized version
- `prompts/gemini.txt` - Gemini-optimized version
- `Builder.ForProvider(providerID, modelID)` - Model-based prompt selection

Current single base.txt is sufficient for now.

## Comparison with opencode

| Feature | opencode | gcode (now) |
|---------|----------|-------------|
| Modular architecture | ✓ | ✓ |
| Detailed behavior guidance | ✓ | ✓ (Go-customized) |
| Environment info injection | ✓ | ✓ |
| Multi-model optimization | ✓ | - (future optional) |
| Task management guidance | ✓ | - (todo tool not implemented) |

## File Changes

**Added**:
- internal/systemprompt/systemprompt.go (58 lines)
- internal/systemprompt/prompts/base.txt (95 lines)
- internal/systemprompt/systemprompt_test.go (62 lines)

**Modified**:
- internal/loop/loop.go (simplified buildSystemPrompt, added isGitRepository)
- memory/MEMORY.md (updated architecture decisions and key files)
