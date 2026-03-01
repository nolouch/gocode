package routes

import (
	"net/http"
	"sort"

	"github.com/nolouch/opengocode/internal/agent"
)

type agentInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Mode        string `json:"mode"`
}

// RegisterAgents mounts a route returning available agents.
func RegisterAgents(mux *http.ServeMux, registry *agent.Registry) {
	if registry == nil {
		return
	}
	mux.HandleFunc("GET /v1/agents", func(w http.ResponseWriter, r *http.Request) {
		list := registry.List()
		out := make([]agentInfo, 0, len(list))
		for _, a := range list {
			if a == nil {
				continue
			}
			out = append(out, agentInfo{
				Name:        a.Name,
				Description: a.Description,
				Mode:        string(a.Mode),
			})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		jsonOK(w, out)
	})
}
