package routes

import (
	"encoding/json"
	"net/http"
	"os"
)

// RegisterConfig mounts config routes onto mux.
func RegisterConfig(mux *http.ServeMux) {
	// GET /config — return basic runtime info
	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		wd, _ := os.Getwd()
		jsonOK(w, map[string]any{
			"work_dir": wd,
			"version":  "0.1.0",
		})
	})

	// Silence unused import warning
	_ = json.Marshal
}
