package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCreateSessionWithParent(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sessions" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ID":        "sess-1",
			"Directory": "/repo",
			"ParentID":  "parent-1",
		})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL})
	_, err := c.CreateSession(context.Background(), "/repo", "parent-1")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	if got["parent_id"] != "parent-1" {
		t.Fatalf("parent_id = %#v, want %q", got["parent_id"], "parent-1")
	}
}

func TestClientListChildSessions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/sessions/parent-1/children" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"ID": "child-1", "ParentID": "parent-1"},
		})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL})
	children, err := c.ListChildSessions(context.Background(), "parent-1")
	if err != nil {
		t.Fatalf("ListChildSessions error: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("children count = %d, want 1", len(children))
	}
	if children[0].ParentID != "parent-1" {
		t.Fatalf("parent id = %q, want parent-1", children[0].ParentID)
	}
}
