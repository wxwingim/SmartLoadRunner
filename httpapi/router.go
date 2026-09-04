package httpapi

import "net/http"

// NewRouter собирает маршруты HTTP API координатора.
func NewRouter(h *Handlers) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tests", h.CreateTest)
	mux.HandleFunc("POST /api/tests/{id}/run", h.StartRun)
	mux.HandleFunc("POST /api/runs/{id}/metrics", h.PostMetrics)
	mux.HandleFunc("GET /api/runs/{id}/stream", h.StreamMetrics)
	mux.HandleFunc("GET /healthz", h.Health)
	mux.HandleFunc("POST /api/agents", h.RegisterAgent)
	mux.HandleFunc("GET /api/runs/{id}/config", h.GetRunConfig)
	return mux
}
