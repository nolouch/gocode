package routes

import (
	"net/http"
	"sort"

	"github.com/nolouch/gcode/internal/tool"
)

type toolInfo struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
}

// RegisterTools mounts a route returning the effective tool definitions.
func RegisterTools(mux *http.ServeMux, tools map[string]tool.Tool) {
	mux.HandleFunc("GET /v1/tools", func(w http.ResponseWriter, r *http.Request) {
		out := make([]toolInfo, 0, len(tools))
		for id, t := range tools {
			out = append(out, toolInfo{
				ID:          id,
				Description: t.Description(),
				Schema:      t.Schema(),
			})
		}
		sort.Slice(out, func(i, j int) bool {
			return out[i].ID < out[j].ID
		})
		jsonOK(w, out)
	})
}
