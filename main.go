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
	"github.com/nolouch/gcode/internal/bus"
	"github.com/nolouch/gcode/internal/config"
	"github.com/nolouch/gcode/internal/llm"
	"github.com/nolouch/gcode/internal/loop"
	"github.com/nolouch/gcode/internal/mcp"
	"github.com/nolouch/gcode/internal/server"
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
	cmd := &cobra.Command{
		Use:   "gcode",
		Short: "gcode — Go coding agent",
	}
	cmd.AddCommand(tuiCmd(), serveCmd(), runCmd(), configCmd())
	return cmd
}

// ── runtime holds all initialized components ──────────────────────────────

type runtime struct {
	cfg       *config.Config
	store     *session.Store
	runner    *loop.Runner
	evBus     *bus.Bus
	mcpMgr    *mcp.Manager
	agentName string
	workDir   string
}

func buildRuntime(ctx context.Context, workDir, agentName string) (*runtime, error) {
	cfg, err := config.Load(workDir)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if cfg.Provider.APIKey == "" {
		return nil, fmt.Errorf("no API key — set OPENAI_API_KEY or provider.api_key in config.yaml")
	}

	store := session.NewStore()
	agents := agent.NewRegistry()
	toolReg := tool.NewRegistry()

	mcpMgr := mcp.NewManager(cfg.MCPServers())
	for _, t := range mcpMgr.Tools(ctx) {
		toolReg.Register(t)
	}

	skills, err := skill.Load(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[warn] skill load error: %v\n", err)
	}
	var skillPrompts []string
	if sp := skill.SystemPrompt(skills); sp != "" {
		skillPrompts = []string{sp}
	}

	lc, err := llm.New(llm.Config{
		ProviderName: cfg.Provider.Name,
		BaseURL:      cfg.Provider.BaseURL,
		APIKey:       cfg.Provider.APIKey,
		Model:        cfg.Provider.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}

	if cfg.DefaultAgent != "" && agentName == "build" {
		agentName = cfg.DefaultAgent
	}

	evBus := bus.New()
	runner := &loop.Runner{
		Store:             store,
		LLM:               lc,
		Agents:            agents,
		Tools:             toolReg.All(),
		SystemPromptExtra: skillPrompts,
		Bus:               evBus,
	}

	return &runtime{
		cfg: cfg, store: store, runner: runner,
		evBus: evBus, mcpMgr: mcpMgr,
		agentName: agentName, workDir: workDir,
	}, nil
}

// ── gcode tui (default interactive mode) ──────────────────────────────────

func tuiCmd() *cobra.Command {
	var (
		workDir   string
		agentName string
		addr      string
	)
	wd, _ := os.Getwd()
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Start interactive TUI (default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			rt, err := buildRuntime(ctx, workDir, agentName)
			if err != nil {
				return err
			}
			defer rt.mcpMgr.Close()

			// Attach terminal fallback printer (until full TUI is implemented)
			bus.SubscribeTerminal(rt.evBus)

			// Start server in background
			srv := server.New(server.Config{Addr: addr}, rt.store, rt.runner, rt.evBus)
			go srv.Listen(ctx)

			// Create initial session
			sess := rt.store.CreateSession(workDir)
			fmt.Printf("gcode  model=%s  agent=%s  dir=%s\n\n", rt.cfg.Provider.Model, rt.agentName, workDir)
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
				if err := rt.runner.Run(ctx, sess.ID, input, rt.agentName); err != nil {
					if err == context.Canceled {
						fmt.Println("\n[interrupted]")
						continue
					}
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&workDir, "dir", "d", wd, "Working directory")
	cmd.Flags().StringVarP(&agentName, "agent", "a", "build", "Agent to use")
	cmd.Flags().StringVar(&addr, "addr", "", "TCP address to expose server (e.g. :4096); empty = Unix socket only")
	return cmd
}

// ── gcode serve (server-only daemon) ──────────────────────────────────────

func serveCmd() *cobra.Command {
	var (
		workDir string
		addr    string
		sock    string
	)
	wd, _ := os.Getwd()
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start agent server (no TUI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			rt, err := buildRuntime(ctx, workDir, "build")
			if err != nil {
				return err
			}
			defer rt.mcpMgr.Close()

			srv := server.New(server.Config{Addr: addr, SocketPath: sock}, rt.store, rt.runner, rt.evBus)
			return srv.Listen(ctx)
		},
	}
	cmd.Flags().StringVarP(&workDir, "dir", "d", wd, "Working directory")
	cmd.Flags().StringVar(&addr, "addr", "", "TCP address (e.g. :4096)")
	cmd.Flags().StringVar(&sock, "socket", "", "Unix socket path (default ~/.gcode/run/gcode.sock)")
	return cmd
}

// ── gcode run (one-shot non-interactive) ──────────────────────────────────

func runCmd() *cobra.Command {
	var (
		workDir   string
		agentName string
		prompt    string
	)
	wd, _ := os.Getwd()
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a single prompt non-interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			if prompt == "" {
				return fmt.Errorf("--prompt/-p is required")
			}
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			rt, err := buildRuntime(ctx, workDir, agentName)
			if err != nil {
				return err
			}
			defer rt.mcpMgr.Close()
			bus.SubscribeTerminal(rt.evBus)

			sess := rt.store.CreateSession(workDir)
			return rt.runner.Run(ctx, sess.ID, prompt, rt.agentName)
		},
	}
	cmd.Flags().StringVarP(&workDir, "dir", "d", wd, "Working directory")
	cmd.Flags().StringVarP(&agentName, "agent", "a", "build", "Agent to use")
	cmd.Flags().StringVarP(&prompt, "prompt", "p", "", "Prompt to run")
	return cmd
}

// ── gcode config ──────────────────────────────────────────────────────────

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
