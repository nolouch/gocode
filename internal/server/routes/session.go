package routes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nolouch/gcode/internal/loop"
	"github.com/nolouch/gcode/internal/session"
)

// RegisterSession mounts session and message routes onto mux.
func RegisterSession(mux *http.ServeMux, store *session.Store, runner *loop.Runner) {
	// POST /session — create a new session
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
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

	// GET /session — list all sessions
	mux.HandleFunc("GET /session", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, store.ListSessions())
	})

	// GET /session/{id} — get a single session
	mux.HandleFunc("GET /session/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sess, err := store.GetSession(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonOK(w, sess)
	})

	// GET /session/{id}/messages — list messages in a session
	mux.HandleFunc("GET /session/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		jsonOK(w, store.Messages(id))
	})

	// POST /session/{id}/message — send a user message, stream agent response via SSE
	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, r *http.Request) {
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

		// Set up SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, canFlush := w.(http.Flusher)

		sendEvent := func(eventType string, payload any) {
			data, _ := json.Marshal(payload)
			w.Write([]byte("event: " + eventType + "\ndata: " + string(data) + "\n\n"))
			if canFlush {
				flusher.Flush()
			}
		}

		// Heartbeat ticker
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		done := make(chan struct{})

		go func() {
			defer close(done)
			err := runner.Run(r.Context(), id, req.Text, req.AgentName)
			if err != nil {
				sendEvent("error", map[string]string{"message": err.Error()})
			}
			sendEvent("done", map[string]string{"status": "ok"})
		}()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				w.Write([]byte(": heartbeat\n\n"))
				if canFlush {
					flusher.Flush()
				}
			case <-r.Context().Done():
				return
			}
		}
	})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
