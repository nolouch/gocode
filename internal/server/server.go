// Package server provides the HTTP server for gcode.
// It exposes REST endpoints and SSE streams so any UI (TUI, web, etc.)
// can connect to the agent backend.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/nolouch/gcode/internal/bus"
	"github.com/nolouch/gcode/internal/loop"
	"github.com/nolouch/gcode/internal/session"
	"github.com/nolouch/gcode/internal/server/routes"
)

// Config holds server startup options.
type Config struct {
	// SocketPath is the Unix socket path for local IPC.
	// If empty, defaults to ~/.gcode/run/gcode.sock
	SocketPath string
	// Addr is a TCP address (e.g. ":4096"). If set, TCP is used instead of Unix socket.
	Addr string
}

// Server wraps the HTTP mux and dependencies.
type Server struct {
	cfg     Config
	handler http.Handler
}

// New creates a Server wiring up all routes.
func New(cfg Config, store *session.Store, runner *loop.Runner, b *bus.Bus) *Server {
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Routes
	routes.RegisterSession(mux, store, runner)
	routes.RegisterEvents(mux, b)
	routes.RegisterConfig(mux)

	return &Server{cfg: cfg, handler: mux}
}

// Listen starts the server and blocks until ctx is cancelled.
func (s *Server) Listen(ctx context.Context) error {
	var ln net.Listener
	var err error

	if s.cfg.Addr != "" {
		ln, err = net.Listen("tcp", s.cfg.Addr)
		if err != nil {
			return fmt.Errorf("server: tcp listen %s: %w", s.cfg.Addr, err)
		}
		fmt.Printf("[server] listening on http://%s\n", s.cfg.Addr)
	} else {
		sockPath := s.cfg.SocketPath
		if sockPath == "" {
			home, _ := os.UserHomeDir()
			sockPath = filepath.Join(home, ".gcode", "run", "gcode.sock")
		}
		if err := os.MkdirAll(filepath.Dir(sockPath), 0o755); err != nil {
			return fmt.Errorf("server: mkdir: %w", err)
		}
		os.Remove(sockPath) // remove stale socket
		ln, err = net.Listen("unix", sockPath)
		if err != nil {
			return fmt.Errorf("server: unix listen %s: %w", sockPath, err)
		}
		fmt.Printf("[server] listening on unix://%s\n", sockPath)
	}

	srv := &http.Server{Handler: s.handler}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
