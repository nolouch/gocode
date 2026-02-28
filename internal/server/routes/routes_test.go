package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nolouch/gcode/internal/agent"
	"github.com/nolouch/gcode/internal/server/runs"
	"github.com/nolouch/gcode/internal/session"
	"github.com/nolouch/gcode/internal/tool"
)

func TestSessionRoutesV1_CreateAndList(t *testing.T) {
	mux := http.NewServeMux()
	store := session.NewStore()
	RegisterSession(mux, store, nil, runs.NewManager())

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

func TestSessionRoutesV1_CreateChildAndListChildren(t *testing.T) {
	mux := http.NewServeMux()
	store := session.NewStore()
	RegisterSession(mux, store, nil, runs.NewManager())

	parentReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"work_dir":"/tmp/project"}`))
	parentRec := httptest.NewRecorder()
	mux.ServeHTTP(parentRec, parentReq)
	if parentRec.Code != http.StatusOK {
		t.Fatalf("create parent status=%d body=%s", parentRec.Code, parentRec.Body.String())
	}
	var parent map[string]any
	if err := json.Unmarshal(parentRec.Body.Bytes(), &parent); err != nil {
		t.Fatalf("decode parent session: %v", err)
	}
	parentID, _ := parent["ID"].(string)

	body := bytes.NewBufferString(`{"work_dir":"/tmp/project","parent_id":"` + parentID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create child status=%d body=%s", rec.Code, rec.Body.String())
	}

	childListReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+parentID+"/children", nil)
	childListRec := httptest.NewRecorder()
	mux.ServeHTTP(childListRec, childListReq)
	if childListRec.Code != http.StatusOK {
		t.Fatalf("list children status=%d body=%s", childListRec.Code, childListRec.Body.String())
	}
	var children []map[string]any
	if err := json.Unmarshal(childListRec.Body.Bytes(), &children); err != nil {
		t.Fatalf("decode children: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child session, got %d", len(children))
	}
}

func TestRunRoutesV1_GetAndAbort(t *testing.T) {
	mux := http.NewServeMux()
	rm := runs.NewManager()
	RegisterRuns(mux, rm)

	r := rm.Start(context.Background(), "sess-1", "hello", "build", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	getReq := httptest.NewRequest(http.MethodGet, "/v1/runs/"+r.ID, nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get run status=%d body=%s", getRec.Code, getRec.Body.String())
	}

	abortReq := httptest.NewRequest(http.MethodPost, "/v1/runs/"+r.ID+"/abort", nil)
	abortRec := httptest.NewRecorder()
	mux.ServeHTTP(abortRec, abortReq)
	if abortRec.Code != http.StatusOK {
		t.Fatalf("abort run status=%d body=%s", abortRec.Code, abortRec.Body.String())
	}

	// Allow goroutine to observe cancellation and settle status.
	time.Sleep(10 * time.Millisecond)
	curr, ok := rm.Get(r.ID)
	if !ok {
		t.Fatalf("run not found")
	}
	if curr.Status != runs.StatusAborted {
		t.Fatalf("expected aborted, got %s", curr.Status)
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

func TestAgentsRouteV1_ListsBuiltins(t *testing.T) {
	mux := http.NewServeMux()
	reg := agent.NewRegistry()
	RegisterAgents(mux, reg)

	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("agents status=%d body=%s", rec.Code, rec.Body.String())
	}

	var defs []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &defs); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	names := map[string]bool{}
	for _, d := range defs {
		if name, ok := d["name"].(string); ok {
			names[name] = true
		}
	}
	for _, required := range []string{"build", "explore", "plan"} {
		if !names[required] {
			t.Fatalf("missing agent %q in /v1/agents", required)
		}
	}
}
