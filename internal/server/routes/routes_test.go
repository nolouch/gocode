package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolouch/gcode/internal/session"
	"github.com/nolouch/gcode/internal/tool"
)

func TestSessionRoutesV1_CreateAndList(t *testing.T) {
	mux := http.NewServeMux()
	store := session.NewStore()
	RegisterSession(mux, store, nil)

	body := bytes.NewBufferString(`{"work_dir":"/tmp/project"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create session status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list sessions status=%d body=%s", rec.Code, rec.Body.String())
	}
	var sessions []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("decode list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

func TestToolsRouteV1_ContainsCoreTools(t *testing.T) {
	mux := http.NewServeMux()
	tools := tool.NewRegistry().All()
	RegisterTools(mux, tools)

	req := httptest.NewRequest(http.MethodGet, "/v1/tools", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tools status=%d body=%s", rec.Code, rec.Body.String())
	}

	var defs []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &defs); err != nil {
		t.Fatalf("decode tools: %v", err)
	}
	ids := map[string]bool{}
	for _, d := range defs {
		if id, ok := d["id"].(string); ok {
			ids[id] = true
		}
	}
	for _, required := range []string{"read", "write", "edit", "glob", "grep", "list", "bash", "apply_patch"} {
		if !ids[required] {
			t.Fatalf("missing tool %q in /v1/tools", required)
		}
	}
}
