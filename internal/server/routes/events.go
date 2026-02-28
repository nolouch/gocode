package routes

import (
	"encoding/json"
	"net/http"

	"github.com/nolouch/gcode/internal/bus"
)

// RegisterEvents mounts the global SSE event stream onto mux.
// GET /events — streams all bus events as SSE to the client.
func RegisterEvents(mux *http.ServeMux, b *bus.Bus) {
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, canFlush := w.(http.Flusher)

		// Send connected event
		w.Write([]byte("event: connected\ndata: {}\n\n"))
		if canFlush {
			flusher.Flush()
		}

		// Subscribe to bus events, forward to SSE
		eventCh := make(chan bus.Event, 64)
		unsub := b.Subscribe(func(e bus.Event) {
			select {
			case eventCh <- e:
			default: // drop if client is slow
			}
		})
		defer unsub()

		for {
			select {
			case e := <-eventCh:
				data, _ := json.Marshal(e)
				w.Write([]byte("event: " + string(e.Type) + "\ndata: " + string(data) + "\n\n"))
				if canFlush {
					flusher.Flush()
				}
			case <-r.Context().Done():
				return
			}
		}
	})
}
