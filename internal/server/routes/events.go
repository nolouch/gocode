package routes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nolouch/opengocode/internal/bus"
)

// RegisterEvents mounts the global SSE event stream onto mux.
// GET /v1/events?session_id=... streams all events, optionally filtered by session.
func RegisterEvents(mux *http.ServeMux, b *bus.Bus) {
	mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, canFlush := w.(http.Flusher)
		sessionID := r.URL.Query().Get("session_id")

		// Send connected event
		w.Write([]byte("event: connected\ndata: {}\n\n"))
		if canFlush {
			flusher.Flush()
		}

		// Subscribe to bus events, forward to SSE
		eventCh := make(chan bus.Event, 64)
		unsub := b.Subscribe(func(e bus.Event) {
			if sessionID != "" && e.SessionID != sessionID {
				return
			}
			select {
			case eventCh <- e:
			default: // drop if client is slow
			}
		})
		defer unsub()
		heartbeat := time.NewTicker(10 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case e := <-eventCh:
				data, _ := json.Marshal(e)
				w.Write([]byte("event: " + string(e.Type) + "\ndata: " + string(data) + "\n\n"))
				if canFlush {
					flusher.Flush()
				}
			case <-heartbeat.C:
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
