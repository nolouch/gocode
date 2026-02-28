package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nolouch/gcode/internal/agent"
	"github.com/nolouch/gcode/internal/bus"
	tui "github.com/nolouch/gcode/internal/cli/tui"
	"github.com/nolouch/gcode/internal/config"
	"github.com/nolouch/gcode/internal/llm"
	"github.com/nolouch/gcode/internal/loop"
	"github.com/nolouch/gcode/internal/mcp"
	"github.com/nolouch/gcode/internal/server"
	"github.com/nolouch/gcode/internal/session"
	"github.com/nolouch/gcode/internal/skill"
	"github.com/nolouch/gcode/internal/storage"
	"github.com/nolouch/gcode/internal/tool"
	"github.com/spf13/cobra"
)

// NewRootCmd builds the CLI command tree.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gcode",
		Short: "gcode — Go coding agent",
	}
	cmd.AddCommand(tuiCmd(), serveCmd(), runCmd(), configCmd())
	return cmd
}

type runtime struct {
	cfg       *config.Config
	store     session.StoreAPI
	runner    *loop.Runner
	evBus     *bus.Bus
	mcpMgr    *mcp.Manager
	db        *storage.DB
	logFile   *os.File
	agentName string
	workDir   string
}

func (r *runtime) Close() {
	if r.mcpMgr != nil {
		r.mcpMgr.Close()
	}
	if r.db != nil {
		r.db.Close()
	}
	if r.logFile != nil {
		r.logFile.Close()
	}
}

func openLoopLogFile() *os.File {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	logDir := filepath.Join(home, ".gcode", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(logDir, "loop.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil
	}
	return f
}

func buildRuntime(ctx context.Context, workDir, agentName string) (*runtime, error) {
	cfg, err := config.Load(workDir)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if cfg.Provider.APIKey == "" {
		return nil, fmt.Errorf("no API key — set OPENAI_API_KEY or provider.api_key in config.yaml")
	}

	db, err := storage.Open(storage.DefaultPath())
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	store, err := storage.NewPersistentStore(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: load: %w", err)
	}
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
	logFile := openLoopLogFile()
	debugEnabled := strings.EqualFold(strings.TrimSpace(os.Getenv("LOG_LEVEL")), "debug")
	llm.SetDebugLogger(func(format string, args ...any) {
		if logFile == nil {
			return
		}
		_, _ = fmt.Fprintf(logFile, "%s [llm][debug] ", time.Now().Format(time.RFC3339))
		_, _ = fmt.Fprintf(logFile, format, args...)
		_, _ = fmt.Fprintln(logFile)
	})
	runner := &loop.Runner{
		Store:             store,
		LLM:               lc,
		Agents:            agents,
		Tools:             toolReg.All(),
		SystemPromptExtra: skillPrompts,
		Bus:               evBus,
		Logf: func(format string, args ...any) {
			if logFile == nil {
				return
			}
			_, _ = fmt.Fprintf(logFile, "%s [loop] ", time.Now().Format(time.RFC3339))
			_, _ = fmt.Fprintf(logFile, format, args...)
		},
		Debug: debugEnabled,
	}

	return &runtime{
		cfg: cfg, store: store, runner: runner,
		evBus: evBus, mcpMgr: mcpMgr, db: db, logFile: logFile,
		agentName: agentName, workDir: workDir,
	}, nil
}

func tuiCmd() *cobra.Command {
	var (
		workDir   string
		agentName string
		addr      string
		sock      string
		attach    bool
	)
	wd, _ := os.Getwd()
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Start interactive TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
			defer cancel()

			if attach {
				return tui.Run(ctx, "remote", agentName, workDir, addr, sock)
			}

			rt, err := buildRuntime(ctx, workDir, agentName)
			if err != nil {
				return err
			}
			defer rt.Close()

			srv := server.New(server.Config{Addr: addr, SocketPath: sock}, rt.store, rt.runner, rt.evBus, rt.runner.Tools)
			go srv.Listen(ctx)

			return tui.Run(ctx, rt.cfg.Provider.Model, rt.agentName, workDir, addr, sock)
		},
	}
	cmd.Flags().StringVarP(&workDir, "dir", "d", wd, "Working directory")
	cmd.Flags().StringVarP(&agentName, "agent", "a", "build", "Agent to use")
	cmd.Flags().StringVar(&addr, "addr", "", "TCP address to expose server (e.g. :4096); empty = Unix socket only")
	cmd.Flags().StringVar(&sock, "socket", "", "Unix socket path (default ~/.gcode/run/gcode.sock)")
	cmd.Flags().BoolVar(&attach, "attach", false, "Attach to an existing server instead of starting an embedded runtime")
	return cmd
}

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
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
			defer cancel()

			rt, err := buildRuntime(ctx, workDir, "build")
			if err != nil {
				return err
			}
			defer rt.Close()

			srv := server.New(server.Config{Addr: addr, SocketPath: sock}, rt.store, rt.runner, rt.evBus, rt.runner.Tools)
			return srv.Listen(ctx)
		},
	}
	cmd.Flags().StringVarP(&workDir, "dir", "d", wd, "Working directory")
	cmd.Flags().StringVar(&addr, "addr", "", "TCP address (e.g. :4096)")
	cmd.Flags().StringVar(&sock, "socket", "", "Unix socket path (default ~/.gcode/run/gcode.sock)")
	return cmd
}

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
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
			defer cancel()

			rt, err := buildRuntime(ctx, workDir, agentName)
			if err != nil {
				return err
			}
			defer rt.Close()
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
