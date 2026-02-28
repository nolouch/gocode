package routes

import (
	"encoding/json"
	"net/http"

	"github.com/nolouch/gcode/internal/loop"
	"github.com/nolouch/gcode/internal/session"
)

// RegisterSession mounts session and message routes onto mux.
func RegisterSession(mux *http.ServeMux, store session.StoreAPI, runner *loop.Runner) {
	// POST /v1/sessions — create a new session
	mux.HandleFunc("POST /v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WorkDir string `json:"work_dir"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.WorkDir == "" {
			req.WorkDir = "."
		}
		sess := store.CreateSession(req.WorkDir)
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

	// POST /v1/sessions/{id}/messages — send a user message.
	// Streaming updates are available via GET /v1/events.
	mux.HandleFunc("POST /v1/sessions/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

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

		if err := runner.Run(r.Context(), id, req.Text, req.AgentName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
