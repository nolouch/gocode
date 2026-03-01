package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nolouch/gocode/internal/tool"
)

// TestSanitize tests the sanitize function for tool naming
func TestSanitize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with-dash", "with-dash"},      // Dashes are preserved
		{"with.dot", "with_dot"},        // Dots become underscores
		{"with space", "with_space"},    // Spaces become underscores
		{"MixedCase", "MixedCase"},      // Case is preserved
		{"multiple---dashes", "multiple---dashes"}, // Multiple dashes preserved
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("sanitize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestMCPTool_ID tests the tool ID generation
func TestMCPTool_ID(t *testing.T) {
	mcpTool := &MCPTool{
		serverName: "test-server",
		name:       "my-tool",
	}

	expected := "test-server_my-tool"
	if got := mcpTool.ID(); got != expected {
		t.Errorf("MCPTool.ID() = %q, want %q", got, expected)
	}
}

// TestMCPTool_Execute tests tool execution
func TestMCPTool_Execute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mcpTool := &MCPTool{
			serverName: "test",
			name:       "echo",
			desc:       "Echo tool",
			schema:     map[string]any{"type": "object"},
			call: func(ctx context.Context, args map[string]any) (string, error) {
				return "success", nil
			},
		}

		ctx := tool.Context{Ctx: context.Background()}
		result, err := mcpTool.Execute(ctx, map[string]any{"message": "hello"})

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		if result.IsError {
			t.Errorf("Result should not be error")
		}
		if result.Output != "success" {
			t.Errorf("Output = %q, want %q", result.Output, "success")
		}
	})

	t.Run("error", func(t *testing.T) {
		mcpTool := &MCPTool{
			serverName: "test",
			name:       "fail",
			call: func(ctx context.Context, args map[string]any) (string, error) {
				return "", fmt.Errorf("test error")
			},
		}

		ctx := tool.Context{Ctx: context.Background()}
		result, err := mcpTool.Execute(ctx, map[string]any{})

		if err != nil {
			t.Fatalf("Execute should not return error: %v", err)
		}
		if !result.IsError {
			t.Errorf("Result should be error")
		}
		if !strings.Contains(result.Output, "test error") {
			t.Errorf("Output should contain error message, got: %s", result.Output)
		}
	})
}

// TestClient_Remote tests remote MCP server communication
func TestClient_Remote(t *testing.T) {
	// Create mock MCP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonrpcReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
			return
		}

		resp := jsonrpcResp{ID: req.ID}

		switch req.Method {
		case "tools/list":
			result := toolListResult{
				Tools: []struct {
					Name        string         `json:"name"`
					Description string         `json:"description"`
					InputSchema map[string]any `json:"inputSchema"`
				}{
					{
						Name:        "test-tool",
						Description: "A test tool",
						InputSchema: map[string]any{"type": "object"},
					},
				},
			}
			resp.Result, _ = json.Marshal(result)

		case "tools/call":
			result := toolCallResult{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{Type: "text", Text: "tool result"},
				},
			}
			resp.Result, _ = json.Marshal(result)

		default:
			resp.Error = &struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}{Code: -32601, Message: "Method not found"}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client
	client := newClient("test-server", ServerConfig{
		Type: ServerTypeRemote,
		URL:  server.URL,
	})

	// Test ListTools
	ctx := context.Background()
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}

	mcpTool := tools[0].(*MCPTool)
	if mcpTool.name != "test-tool" {
		t.Errorf("Tool name = %q, want %q", mcpTool.name, "test-tool")
	}
	if mcpTool.desc != "A test tool" {
		t.Errorf("Tool description = %q, want %q", mcpTool.desc, "A test tool")
	}

	// Test tool execution
	toolCtx := tool.Context{Ctx: ctx}
	result, err := mcpTool.Execute(toolCtx, map[string]any{"arg": "value"})
	if err != nil {
		t.Fatalf("Tool execution failed: %v", err)
	}
	if result.IsError {
		t.Errorf("Result should not be error")
	}
	if !strings.Contains(result.Output, "tool result") {
		t.Errorf("Output should contain 'tool result', got: %s", result.Output)
	}
}

// TestClient_RemoteWithHeaders tests custom headers
func TestClient_RemoteWithHeaders(t *testing.T) {
	receivedHeaders := make(map[string]string)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture headers
		receivedHeaders["Authorization"] = r.Header.Get("Authorization")
		receivedHeaders["X-Custom"] = r.Header.Get("X-Custom")

		var req jsonrpcReq
		json.NewDecoder(r.Body).Decode(&req)

		resp := jsonrpcResp{
			ID:     req.ID,
			Result: json.RawMessage(`{"tools":[]}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newClient("test", ServerConfig{
		Type: ServerTypeRemote,
		URL:  server.URL,
		Headers: map[string]string{
			"Authorization": "Bearer token123",
			"X-Custom":      "custom-value",
		},
	})

	ctx := context.Background()
	_, _ = client.ListTools(ctx)

	if receivedHeaders["Authorization"] != "Bearer token123" {
		t.Errorf("Authorization header = %q, want %q", receivedHeaders["Authorization"], "Bearer token123")
	}
	if receivedHeaders["X-Custom"] != "custom-value" {
		t.Errorf("X-Custom header = %q, want %q", receivedHeaders["X-Custom"], "custom-value")
	}
}

// TestClient_Timeout tests timeout configuration
func TestClient_Timeout(t *testing.T) {
	client := newClient("test", ServerConfig{
		Type:      ServerTypeRemote,
		TimeoutMs: 5000,
	})

	timeout := client.timeout()
	expected := 5 * time.Second
	if timeout != expected {
		t.Errorf("timeout() = %v, want %v", timeout, expected)
	}

	// Test default timeout
	client2 := newClient("test2", ServerConfig{
		Type: ServerTypeRemote,
	})
	timeout2 := client2.timeout()
	expected2 := 30 * time.Second
	if timeout2 != expected2 {
		t.Errorf("default timeout() = %v, want %v", timeout2, expected2)
	}
}

// TestManager_Tools tests the Manager's tool aggregation
func TestManager_Tools(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonrpcReq
		json.NewDecoder(r.Body).Decode(&req)

		resp := jsonrpcResp{ID: req.ID}
		if req.Method == "tools/list" {
			result := toolListResult{
				Tools: []struct {
					Name        string         `json:"name"`
					Description string         `json:"description"`
					InputSchema map[string]any `json:"inputSchema"`
				}{
					{Name: "tool1", Description: "Tool 1"},
					{Name: "tool2", Description: "Tool 2"},
				},
			}
			resp.Result, _ = json.Marshal(result)
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create manager with test config
	configs := map[string]ServerConfig{
		"server1": {
			Type:    ServerTypeRemote,
			URL:     server.URL,
			Enabled: true,
		},
		"server2": {
			Type:    ServerTypeRemote,
			URL:     server.URL,
			Enabled: false, // Disabled
		},
	}

	mgr := NewManager(configs)
	ctx := context.Background()
	tools := mgr.Tools(ctx)

	// Should have 2 tools from server1 (server2 is disabled)
	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}

	// Check tool IDs
	expectedIDs := map[string]bool{
		"server1_tool1": true,
		"server1_tool2": true,
	}

	for _, tool := range tools {
		id := tool.ID()
		if !expectedIDs[id] {
			t.Errorf("Unexpected tool ID: %s", id)
		}
		delete(expectedIDs, id)
	}

	if len(expectedIDs) > 0 {
		t.Errorf("Missing tools: %v", expectedIDs)
	}
}

// TestServerConfig_Validation tests configuration validation
func TestServerConfig_Validation(t *testing.T) {
	t.Run("local without command", func(t *testing.T) {
		client := newClient("test", ServerConfig{
			Type:    ServerTypeLocal,
			Command: []string{},
		})

		err := client.Connect()
		if err == nil {
			t.Error("Expected error for local server without command")
		}
		if !strings.Contains(err.Error(), "no command") {
			t.Errorf("Error should mention 'no command', got: %v", err)
		}
	})
}

// TestJSONRPCError tests error handling
func TestJSONRPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonrpcReq
		json.NewDecoder(r.Body).Decode(&req)

		resp := jsonrpcResp{
			ID: req.ID,
			Error: &struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}{
				Code:    -32600,
				Message: "Invalid Request",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newClient("test", ServerConfig{
		Type: ServerTypeRemote,
		URL:  server.URL,
	})

	ctx := context.Background()
	_, err := client.ListTools(ctx)

	if err == nil {
		t.Fatal("Expected error from JSONRPC error response")
	}

	if !strings.Contains(err.Error(), "Invalid Request") {
		t.Errorf("Error should contain 'Invalid Request', got: %v", err)
	}
}

// TestToolCallResult_MultipleContent tests handling multiple content items
func TestToolCallResult_MultipleContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonrpcReq
		json.NewDecoder(r.Body).Decode(&req)

		resp := jsonrpcResp{ID: req.ID}

		if req.Method == "tools/list" {
			result := toolListResult{
				Tools: []struct {
					Name        string         `json:"name"`
					Description string         `json:"description"`
					InputSchema map[string]any `json:"inputSchema"`
				}{
					{Name: "multi-content", Description: "Test"},
				},
			}
			resp.Result, _ = json.Marshal(result)
		} else if req.Method == "tools/call" {
			result := toolCallResult{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{Type: "text", Text: "Part 1"},
					{Type: "text", Text: "Part 2"},
					{Type: "text", Text: "Part 3"},
				},
			}
			resp.Result, _ = json.Marshal(result)
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newClient("test", ServerConfig{
		Type: ServerTypeRemote,
		URL:  server.URL,
	})

	ctx := context.Background()
	tools, _ := client.ListTools(ctx)
	mcpTool := tools[0].(*MCPTool)

	toolCtx := tool.Context{Ctx: ctx}
	result, err := mcpTool.Execute(toolCtx, map[string]any{})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Should concatenate all content parts
	expected := "Part 1\nPart 2\nPart 3"
	if result.Output != expected {
		t.Errorf("Output = %q, want %q", result.Output, expected)
	}
}
