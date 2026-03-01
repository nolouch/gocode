package routes

import (
	"net/http"

	"github.com/nolouch/opengocode/internal/server/runs"
)

// RegisterRuns mounts run lifecycle routes onto mux.
func RegisterRuns(mux *http.ServeMux, runMgr *runs.Manager) {
	mux.HandleFunc("GET /v1/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		run, ok := runMgr.Get(id)
		if !ok {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		jsonOK(w, run)
	})

	mux.HandleFunc("POST /v1/runs/{id}/abort", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		run, err := runMgr.Abort(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonOK(w, run)
	})
}
