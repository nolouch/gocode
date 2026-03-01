package routes

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/nolouch/opengocode/internal/loop"
	"github.com/nolouch/opengocode/internal/server/runs"
	"github.com/nolouch/opengocode/internal/session"
)

// RegisterSession mounts session and message routes onto mux.
func RegisterSession(mux *http.ServeMux, store session.StoreAPI, runner *loop.Runner, runMgr *runs.Manager) {
	// POST /v1/sessions — create a new session
	mux.HandleFunc("POST /v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WorkDir  string `json:"work_dir"`
			ParentID string `json:"parent_id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.WorkDir == "" {
			req.WorkDir = "."
		}
		if req.ParentID != "" {
			if _, err := store.GetSession(req.ParentID); err != nil {
				http.Error(w, "parent session not found", http.StatusBadRequest)
				return
			}
		}
		sess := store.CreateSession(req.WorkDir)
		if req.ParentID != "" {
			store.SetSessionParent(sess.ID, req.ParentID)
			sess.ParentID = req.ParentID
		}
		jsonOK(w, sess)
	})

	// GET /v1/sessions — list all sessions
	mux.HandleFunc("GET /v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, store.ListSessions())
	})

	// GET /v1/sessions/{id} — get a single session
	mux.HandleFunc("GET /v1/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sess, err := store.GetSession(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonOK(w, sess)
	})

	// GET /v1/sessions/{id}/messages — list messages in a session
	mux.HandleFunc("GET /v1/sessions/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		jsonOK(w, store.Messages(id))
	})

	// GET /v1/sessions/{id}/children — list child sessions for a parent session
	mux.HandleFunc("GET /v1/sessions/{id}/children", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, err := store.GetSession(id); err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		jsonOK(w, store.Children(id))
	})

	// POST /v1/sessions/{id}/messages — send a user message.
	// Streaming updates are available via GET /v1/events and run status APIs.
	mux.HandleFunc("POST /v1/sessions/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if runner == nil || runMgr == nil {
			http.Error(w, "runner not configured", http.StatusInternalServerError)
			return
		}

		var req struct {
			Text      string `json:"text"`
			AgentName string `json:"agent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
			http.Error(w, "missing 'text'", http.StatusBadRequest)
			return
		}
		if req.AgentName == "" {
			req.AgentName = "build"
		}

		// Verify session exists
		if _, err := store.GetSession(id); err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		run := runMgr.Start(context.Background(), id, req.Text, req.AgentName, func(ctx context.Context) error {
			return runner.Run(ctx, id, req.Text, req.AgentName)
		})
		jsonOK(w, run)
	})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
