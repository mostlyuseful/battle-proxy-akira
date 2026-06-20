package api

import (
	"encoding/json"
	"net/http"
)

// NewServer builds the HTTP handler tree for the proxy API.
func NewServer() http.Handler {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux)
	return mux
}

// RegisterHealthRoutes wires the base health and readiness endpoints.
func RegisterHealthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /readyz", readyz)
}

type healthResponse struct {
	Status string `json:"status"`
}

func healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func readyz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ready"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
