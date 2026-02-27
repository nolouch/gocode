// Package config defines gcode's YAML/env configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nolouch/gcode/internal/mcp"
	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure.
// It is read from $HOME/.config/gcode/config.yaml or .gcode/config.yaml in the workspace.
type Config struct {
	// LLM provider settings
	Provider ProviderConfig `yaml:"provider"`

	// MCP server definitions
	MCP map[string]MCPConfig `yaml:"mcp"`

	// Skill paths (additional dirs to scan for SKILL.md files)
	Skills SkillsConfig `yaml:"skills"`

	// Default agent name (default: "build")
	DefaultAgent string `yaml:"default_agent"`
}

// ProviderConfig holds LLM API settings.
type ProviderConfig struct {
	BaseURL string `yaml:"base_url"` // default: https://api.openai.com/v1
	APIKey  string `yaml:"api_key"`  // overridden by OPENAI_API_KEY env var
	Model   string `yaml:"model"`    // default: gpt-4o
}

// MCPConfig mirrors OpenCode's Mcp config entry.
type MCPConfig struct {
	Type    string            `yaml:"type"`    // "local" or "remote"
	Command []string          `yaml:"command"` // local: command + args
	URL     string            `yaml:"url"`     // remote: HTTP URL
	Headers map[string]string `yaml:"headers"`
	Env     map[string]string `yaml:"env"`
	Timeout int               `yaml:"timeout_ms"` // ms
	Enabled *bool             `yaml:"enabled"`
}

// SkillsConfig holds additional skill search paths.
type SkillsConfig struct {
	Paths []string `yaml:"paths"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Provider: ProviderConfig{
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o",
		},
		DefaultAgent: "build",
	}
}

// Load reads configuration from standard locations, merging in env vars.
func Load(workDir string) (*Config, error) {
	cfg := Default()

	candidates := []string{
		filepath.Join(workDir, ".gcode", "config.yaml"),
		filepath.Join(workDir, ".opencode", "config.yaml"),
		filepath.Join(os.Getenv("HOME"), ".config", "gcode", "config.yaml"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
		break
	}

	// Env overrides
	if v := os.Getenv("OPENAI_API_KEY"); v != "" && cfg.Provider.APIKey == "" {
		cfg.Provider.APIKey = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" && cfg.Provider.APIKey == "" {
		cfg.Provider.APIKey = v
	}
	if v := os.Getenv("GCODE_MODEL"); v != "" {
		cfg.Provider.Model = v
	}
	if v := os.Getenv("GCODE_BASE_URL"); v != "" {
		cfg.Provider.BaseURL = v
	}
	// Trim trailing slash from base URL
	cfg.Provider.BaseURL = strings.TrimRight(cfg.Provider.BaseURL, "/")

	return cfg, nil
}

// MCPServers converts cfg.MCP to mcp.ServerConfig entries.
func (cfg *Config) MCPServers() map[string]mcp.ServerConfig {
	out := make(map[string]mcp.ServerConfig)
	for name, m := range cfg.MCP {
		enabled := true
		if m.Enabled != nil {
			enabled = *m.Enabled
		}
		sc := mcp.ServerConfig{
			Enabled:   enabled,
			TimeoutMs: m.Timeout,
			Headers:   m.Headers,
			Env:       m.Env,
		}
		switch m.Type {
		case "local":
			sc.Type = mcp.ServerTypeLocal
			sc.Command = m.Command
		case "remote":
			sc.Type = mcp.ServerTypeRemote
			sc.URL = m.URL
		default:
			continue
		}
		out[name] = sc
	}
	return out
}

// Print dumps the effective config (redacting API key) to stdout.
func (cfg *Config) Print() {
	masked := *cfg
	if masked.Provider.APIKey != "" {
		masked.Provider.APIKey = masked.Provider.APIKey[:4] + "****"
	}
	b, _ := json.MarshalIndent(masked, "", "  ")
	fmt.Println(string(b))
}
