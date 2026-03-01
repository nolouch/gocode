// Package mcp provides a client for the Model Context Protocol.
// Supports local (stdio) and remote (SSE/HTTP) MCP servers,
// mirroring OpenCode's MCP namespace.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/nolouch/gocode/internal/tool"
)

// ServerType distinguishes local (stdio) from remote (HTTP/SSE) servers.
type ServerType string

const (
	ServerTypeLocal  ServerType = "local"
	ServerTypeRemote ServerType = "remote"
)

// ServerConfig is the configuration for a single MCP server.
type ServerConfig struct {
	Type ServerType
	// Local: command + args
	Command []string
	Env     map[string]string
	// Remote: URL
	URL     string
	Headers map[string]string
	// OAuth configuration
	OAuth *OAuthConfig
	// Timeout for connecting / listing tools (default 30s)
	TimeoutMs int
	Enabled   bool
}

// OAuthConfig contains OAuth 2.0 configuration for a remote MCP server.
type OAuthConfig struct {
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Enabled      bool   `json:"enabled"` // Default true if OAuth config exists
}

// MCPTool is a tool exposed by an MCP server.
type MCPTool struct {
	serverName string
	name       string
	desc       string
	schema     map[string]any
	call       func(ctx context.Context, args map[string]any) (string, error)
}

func (t *MCPTool) ID() string             { return t.serverName + "_" + sanitize(t.name) }
func (t *MCPTool) Description() string    { return t.desc }
func (t *MCPTool) Schema() map[string]any { return t.schema }
func (t *MCPTool) Execute(ctx tool.Context, args map[string]any) (tool.Result, error) {
	out, err := t.call(ctx.Ctx, args)
	if err != nil {
		return tool.Result{IsError: true, Output: err.Error()}, nil
	}
	return tool.Result{Output: out, Title: t.name}, nil
}

// ─────────────────────────────────────────────
// JSONRPC helpers
// ─────────────────────────────────────────────

type jsonrpcReq struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type jsonrpcResp struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type toolListResult struct {
	Tools []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"inputSchema"`
	} `json:"tools"`
}

type toolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

type promptListResult struct {
	Prompts []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Arguments   []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Required    bool   `json:"required"`
		} `json:"arguments,omitempty"`
	} `json:"prompts"`
}

type promptGetResult struct {
	Description string `json:"description,omitempty"`
	Messages    []struct {
		Role    string `json:"role"`
		Content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"messages"`
}

type resourceListResult struct {
	Resources []struct {
		URI         string `json:"uri"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		MimeType    string `json:"mimeType,omitempty"`
	} `json:"resources"`
}

type resourceReadResult struct {
	Contents []struct {
		URI      string `json:"uri"`
		MimeType string `json:"mimeType,omitempty"`
		Text     string `json:"text,omitempty"`
		Blob     string `json:"blob,omitempty"`
	} `json:"contents"`
}


// ─────────────────────────────────────────────
// Client
// ─────────────────────────────────────────────

// Client wraps a single MCP server connection.
type Client struct {
	name   string
	cfg    ServerConfig
	mu     sync.Mutex
	nextID int

	// local process
	proc   *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	// remote
	httpClient *http.Client
}

func newClient(name string, cfg ServerConfig) *Client {
	return &Client{
		name:       name,
		cfg:        cfg,
		nextID:     1,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) timeout() time.Duration {
	if c.cfg.TimeoutMs > 0 {
		return time.Duration(c.cfg.TimeoutMs) * time.Millisecond
	}
	return 30 * time.Second
}

// Connect starts/connects the MCP server.
func (c *Client) Connect() error {
	if c.cfg.Type == ServerTypeLocal {
		return c.connectLocal()
	}
	return nil // remote servers are stateless HTTP calls
}

func (c *Client) connectLocal() error {
	if len(c.cfg.Command) == 0 {
		return fmt.Errorf("local MCP server %s has no command", c.name)
	}
	cmd, args := c.cfg.Command[0], c.cfg.Command[1:]
	c.proc = exec.Command(cmd, args...)
	env := os.Environ()
	for k, v := range c.cfg.Env {
		env = append(env, k+"="+v)
	}
	c.proc.Env = env
	c.proc.Stderr = os.Stderr

	stdin, err := c.proc.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := c.proc.StdoutPipe()
	if err != nil {
		return err
	}
	c.stdin = stdin
	c.stdout = bufio.NewReader(stdout)

	if err := c.proc.Start(); err != nil {
		return fmt.Errorf("start MCP server %s: %w", c.name, err)
	}

	// MCP handshake: initialize
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	_, err = c.callLocal(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "gcode", "version": "0.1.0"},
	})
	if err != nil {
		return fmt.Errorf("MCP init handshake for %s: %w", c.name, err)
	}
	// send initialized notification
	notif := jsonrpcReq{JSONRPC: "2.0", Method: "notifications/initialized"}
	b, _ := json.Marshal(notif)
	_, _ = fmt.Fprintf(c.stdin, "%s\n", b)
	return nil
}

// Close shuts down the server.
func (c *Client) Close() {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.proc != nil {
		_ = c.proc.Wait()
	}
}

// ListTools returns tools exposed by this server as tool.Tool implementations.
func (c *Client) ListTools(ctx context.Context) ([]tool.Tool, error) {
	var raw json.RawMessage
	var err error
	if c.cfg.Type == ServerTypeLocal {
		raw, err = c.callLocal(ctx, "tools/list", nil)
	} else {
		raw, err = c.callRemote(ctx, "tools/list", nil)
	}
	if err != nil {
		return nil, err
	}
	var result toolListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}

	out := make([]tool.Tool, 0, len(result.Tools))
	for _, t := range result.Tools {
		t := t // capture
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, &MCPTool{
			serverName: c.name,
			name:       t.Name,
			desc:       t.Description,
			schema:     schema,
			call: func(ctx context.Context, args map[string]any) (string, error) {
				return c.callTool(ctx, t.Name, args)
			},
		})
	}
	return out, nil
}

func (c *Client) callTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	params := map[string]any{"name": toolName, "arguments": args}
	var raw json.RawMessage
	var err error
	if c.cfg.Type == ServerTypeLocal {
		raw, err = c.callLocal(ctx, "tools/call", params)
	} else {
		raw, err = c.callRemote(ctx, "tools/call", params)
	}
	if err != nil {
		return "", err
	}
	var result toolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return string(raw), nil
	}
	var parts []string
	for _, c := range result.Content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}

// ─── Local (stdio) ───────────────────────────

func (c *Client) callLocal(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	req := jsonrpcReq{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	b, _ := json.Marshal(req)
	_, err := fmt.Fprintf(c.stdin, "%s\n", b)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}

	// Read response lines until we get our ID back
	done := make(chan struct {
		raw json.RawMessage
		err error
	}, 1)
	go func() {
		for {
			line, err := c.stdout.ReadString('\n')
			if err != nil {
				done <- struct {
					raw json.RawMessage
					err error
				}{nil, err}
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var resp jsonrpcResp
			if err := json.Unmarshal([]byte(line), &resp); err != nil {
				continue
			}
			if resp.ID != id {
				continue
			}
			if resp.Error != nil {
				done <- struct {
					raw json.RawMessage
					err error
				}{nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)}
				return
			}
			done <- struct {
				raw json.RawMessage
				err error
			}{resp.Result, nil}
			return
		}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		return r.raw, r.err
	}
}

// ─── Remote (HTTP POST) ──────────────────────

func (c *Client) callRemote(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	req := jsonrpcReq{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Add custom headers
	for k, v := range c.cfg.Headers {
		httpReq.Header.Set(k, v)
	}

	// Add OAuth Bearer token if configured
	if c.cfg.OAuth != nil && c.cfg.OAuth.Enabled {
		// Try to get access token
		token, err := Authenticate(c.name, c.cfg)
		if err != nil {
			return nil, fmt.Errorf("OAuth authentication: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var rpcResp jsonrpcResp
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

// ─────────────────────────────────────────────
// Manager — manages multiple MCP servers
// ─────────────────────────────────────────────

// Manager connects to multiple MCP servers and exposes their combined tool set.
type Manager struct {
	clients []*Client
}

// NewManager creates a Manager and connects all enabled servers.
func NewManager(configs map[string]ServerConfig) *Manager {
	m := &Manager{}
	for name, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		c := newClient(name, cfg)
		if err := c.Connect(); err != nil {
			fmt.Fprintf(os.Stderr, "[mcp] failed to connect %s: %v\n", name, err)
			continue
		}
		m.clients = append(m.clients, c)
	}
	return m
}

// Tools returns all MCP tools from connected servers, registered into the given registry.
func (m *Manager) Tools(ctx context.Context) []tool.Tool {
	var all []tool.Tool
	for _, c := range m.clients {
		tools, err := c.ListTools(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mcp] list tools from %s: %v\n", c.name, err)
			continue
		}
		all = append(all, tools...)
	}
	return all
}

// Close shuts down all MCP clients.
func (m *Manager) Close() {
	for _, c := range m.clients {
		c.Close()
	}
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// ListPrompts returns prompts exposed by this server.
func (c *Client) ListPrompts(ctx context.Context) (*promptListResult, error) {
	var raw json.RawMessage
	var err error
	if c.cfg.Type == ServerTypeLocal {
		raw, err = c.callLocal(ctx, "prompts/list", nil)
	} else {
		raw, err = c.callRemote(ctx, "prompts/list", nil)
	}
	if err != nil {
		return nil, err
	}
	var result promptListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPrompt retrieves a specific prompt with optional arguments.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*promptGetResult, error) {
	params := map[string]any{"name": name}
	if len(args) > 0 {
		params["arguments"] = args
	}

	var raw json.RawMessage
	var err error
	if c.cfg.Type == ServerTypeLocal {
		raw, err = c.callLocal(ctx, "prompts/get", params)
	} else {
		raw, err = c.callRemote(ctx, "prompts/get", params)
	}
	if err != nil {
		return nil, err
	}
	var result promptGetResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListResources returns resources exposed by this server.
func (c *Client) ListResources(ctx context.Context) (*resourceListResult, error) {
	var raw json.RawMessage
	var err error
	if c.cfg.Type == ServerTypeLocal {
		raw, err = c.callLocal(ctx, "resources/list", nil)
	} else {
		raw, err = c.callRemote(ctx, "resources/list", nil)
	}
	if err != nil {
		return nil, err
	}
	var result resourceListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ReadResource reads a specific resource by URI.
func (c *Client) ReadResource(ctx context.Context, uri string) (*resourceReadResult, error) {
	params := map[string]any{"uri": uri}

	var raw json.RawMessage
	var err error
	if c.cfg.Type == ServerTypeLocal {
		raw, err = c.callLocal(ctx, "resources/read", params)
	} else {
		raw, err = c.callRemote(ctx, "resources/read", params)
	}
	if err != nil {
		return nil, err
	}
	var result resourceReadResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
