package routes

import (
	"net/http"
	"os"
)

// RegisterConfig mounts config routes onto mux.
func RegisterConfig(mux *http.ServeMux) {
	// GET /v1/config — return basic runtime info
	mux.HandleFunc("GET /v1/config", func(w http.ResponseWriter, r *http.Request) {
		wd, _ := os.Getwd()
		jsonOK(w, map[string]any{
			"work_dir": wd,
			"version":  "0.1.0",
		})
	})
}
