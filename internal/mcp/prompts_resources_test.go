package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClient_ListPrompts tests prompts listing
func TestClient_ListPrompts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonrpcReq
		json.NewDecoder(r.Body).Decode(&req)

		resp := jsonrpcResp{ID: req.ID}
		if req.Method == "prompts/list" {
			result := promptListResult{
				Prompts: []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Arguments   []struct {
						Name        string `json:"name"`
						Description string `json:"description"`
						Required    bool   `json:"required"`
					} `json:"arguments,omitempty"`
				}{
					{
						Name:        "test-prompt",
						Description: "A test prompt",
					},
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
	result, err := client.ListPrompts(ctx)

	if err != nil {
		t.Fatalf("ListPrompts failed: %v", err)
	}
	if len(result.Prompts) != 1 {
		t.Fatalf("Expected 1 prompt, got %d", len(result.Prompts))
	}
	if result.Prompts[0].Name != "test-prompt" {
		t.Errorf("Prompt name = %q, want %q", result.Prompts[0].Name, "test-prompt")
	}
}

// TestClient_GetPrompt tests prompt retrieval
func TestClient_GetPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonrpcReq
		json.NewDecoder(r.Body).Decode(&req)

		resp := jsonrpcResp{ID: req.ID}
		if req.Method == "prompts/get" {
			result := promptGetResult{
				Description: "Test prompt",
				Messages: []struct {
					Role    string `json:"role"`
					Content struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				}{
					{
						Role: "user",
						Content: struct {
							Type string `json:"type"`
							Text string `json:"text"`
						}{Type: "text", Text: "Hello"},
					},
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
	result, err := client.GetPrompt(ctx, "test-prompt", nil)

	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}
	if result.Description != "Test prompt" {
		t.Errorf("Description = %q, want %q", result.Description, "Test prompt")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(result.Messages))
	}
}

// TestClient_ListResources tests resources listing
func TestClient_ListResources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonrpcReq
		json.NewDecoder(r.Body).Decode(&req)

		resp := jsonrpcResp{ID: req.ID}
		if req.Method == "resources/list" {
			result := resourceListResult{
				Resources: []struct {
					URI         string `json:"uri"`
					Name        string `json:"name"`
					Description string `json:"description,omitempty"`
					MimeType    string `json:"mimeType,omitempty"`
				}{
					{
						URI:         "file:///test.txt",
						Name:        "test.txt",
						Description: "A test file",
						MimeType:    "text/plain",
					},
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
	result, err := client.ListResources(ctx)

	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("Expected 1 resource, got %d", len(result.Resources))
	}
	if result.Resources[0].URI != "file:///test.txt" {
		t.Errorf("Resource URI = %q, want %q", result.Resources[0].URI, "file:///test.txt")
	}
}

// TestClient_ReadResource tests resource reading
func TestClient_ReadResource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonrpcReq
		json.NewDecoder(r.Body).Decode(&req)

		resp := jsonrpcResp{ID: req.ID}
		if req.Method == "resources/read" {
			result := resourceReadResult{
				Contents: []struct {
					URI      string `json:"uri"`
					MimeType string `json:"mimeType,omitempty"`
					Text     string `json:"text,omitempty"`
					Blob     string `json:"blob,omitempty"`
				}{
					{
						URI:      "file:///test.txt",
						MimeType: "text/plain",
						Text:     "Hello, World!",
					},
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
	result, err := client.ReadResource(ctx, "file:///test.txt")

	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("Expected 1 content, got %d", len(result.Contents))
	}
	if result.Contents[0].Text != "Hello, World!" {
		t.Errorf("Content text = %q, want %q", result.Contents[0].Text, "Hello, World!")
	}
}
