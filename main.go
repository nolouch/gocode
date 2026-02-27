package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nolouch/gcode/internal/agent"
	"github.com/nolouch/gcode/internal/config"
	"github.com/nolouch/gcode/internal/llm"
	"github.com/nolouch/gcode/internal/loop"
	"github.com/nolouch/gcode/internal/mcp"
	"github.com/nolouch/gcode/internal/session"
	"github.com/nolouch/gcode/internal/skill"
	"github.com/nolouch/gcode/internal/tool"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var (
		agentName string
		workDir   string
		oneShot   string
	)

	cmd := &cobra.Command{
		Use:   "gcode",
		Short: "gcode — Go coding agent",
		Long: `gcode is a terminal-based AI coding agent.

It uses an LLM to read/write files, run bash commands, and call
MCP (Model Context Protocol) servers. Skills (SKILL.md files) are
automatically loaded from standard directories to extend the system prompt.

Environment variables:
  OPENAI_API_KEY   API key for OpenAI-compatible providers
  GCODE_MODEL      Model name (default: gpt-4o)
  GCODE_BASE_URL   Provider base URL (default: https://api.openai.com/v1)
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(workDir, agentName, oneShot)
		},
	}

	wd, _ := os.Getwd()
	cmd.Flags().StringVarP(&workDir, "dir", "d", wd, "Working directory")
	cmd.Flags().StringVarP(&agentName, "agent", "a", "build", "Agent to use (build, explore, ...)")
	cmd.Flags().StringVarP(&oneShot, "prompt", "p", "", "Run a single prompt non-interactively and exit")

	// Sub-commands
	cmd.AddCommand(configCmd())

	return cmd
}

func run(workDir, agentName, oneShot string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// ── Load config ────────────────────────────────────────────────
	cfg, err := config.Load(workDir)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.Provider.APIKey == "" {
		return fmt.Errorf("no API key found — set OPENAI_API_KEY or configure provider.api_key in config.yaml")
	}

	// ── Build component instances ──────────────────────────────────
	store := session.NewStore()
	agents := agent.NewRegistry()
	toolReg := tool.NewRegistry()

	// MCP tools
	mcpMgr := mcp.NewManager(cfg.MCPServers())
	defer mcpMgr.Close()

	mcpTools := mcpMgr.Tools(ctx)
	for _, t := range mcpTools {
		toolReg.Register(t)
	}

	// Skills → system prompt fragments
	skills, err := skill.Load(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[warn] skill load error: %v\n", err)
	}
	var skillPrompts []string
	if sp := skill.SystemPrompt(skills); sp != "" {
		skillPrompts = []string{sp}
	}

	// LLM client
	lc, err := llm.New(llm.Config{
		ProviderName: cfg.Provider.Name,
		BaseURL:      cfg.Provider.BaseURL,
		APIKey:       cfg.Provider.APIKey,
		Model:        cfg.Provider.Model,
	})
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}

	// Agent loop runner
	runner := &loop.Runner{
		Store:             store,
		LLM:               lc,
		Agents:            agents,
		Tools:             toolReg.All(),
		SystemPromptExtra: skillPrompts,
	}

	// Create a session
	sess := store.CreateSession(workDir)

	// Use the configured or overridden default agent
	if cfg.DefaultAgent != "" && agentName == "build" {
		agentName = cfg.DefaultAgent
	}

	fmt.Printf("gcode  model=%s  agent=%s  dir=%s\n", cfg.Provider.Model, agentName, workDir)
	if len(skills) > 0 {
		fmt.Printf("skills: %s\n", skillNames(skills))
	}
	if len(mcpTools) > 0 {
		fmt.Printf("mcp tools: %d loaded\n", len(mcpTools))
	}
	fmt.Println()

	// ── One-shot mode ──────────────────────────────────────────────
	if oneShot != "" {
		return runner.Run(ctx, sess.ID, oneShot, agentName)
	}

	// ── Interactive REPL ───────────────────────────────────────────
	fmt.Println("Type your message (Ctrl-D or 'exit' to quit):")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}
		if err := runner.Run(ctx, sess.ID, input, agentName); err != nil {
			if err == context.Canceled {
				fmt.Println("\n[interrupted]")
				continue
			}
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}
	return nil
}

func configCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print effective configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, _ := os.Getwd()
			cfg, err := config.Load(wd)
			if err != nil {
				return err
			}
			cfg.Print()
			return nil
		},
	}
}

func skillNames(skills []*skill.Info) string {
	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}
