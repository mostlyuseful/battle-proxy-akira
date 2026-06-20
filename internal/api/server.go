package api

import (
	"encoding/json"
	"net/http"

	"battle-proxy-akira/internal/router"
)

// Middleware wraps an HTTP handler. Client authentication can be supplied this way.
type Middleware func(http.Handler) http.Handler

// Option configures the API server.
type Option func(*serverOptions)

type serverOptions struct {
	modelLister ModelLister
	chatRouter  router.Router
	clientAuth  Middleware
}

// WithModelLister configures the source used by GET /v1/models.
func WithModelLister(lister ModelLister) Option {
	return func(opts *serverOptions) {
		opts.modelLister = lister
	}
}

// WithChatRouter configures the router used by POST /v1/chat/completions.
func WithChatRouter(chatRouter router.Router) Option {
	return func(opts *serverOptions) {
		opts.chatRouter = chatRouter
	}
}

// WithClientAuth configures middleware applied to client API routes.
func WithClientAuth(middleware Middleware) Option {
	return func(opts *serverOptions) {
		opts.clientAuth = middleware
	}
}

// NewServer builds the HTTP handler tree for the proxy API.
func NewServer(options ...Option) http.Handler {
	opts := serverOptions{
		modelLister: ModelListerFunc(emptyModels),
		clientAuth:  identityMiddleware,
	}
	for _, option := range options {
		option(&opts)
	}
	if opts.modelLister == nil {
		opts.modelLister = ModelListerFunc(emptyModels)
	}
	if opts.clientAuth == nil {
		opts.clientAuth = identityMiddleware
	}

	mux := http.NewServeMux()
	RegisterHealthRoutes(mux)
	RegisterModelRoutes(mux, opts.modelLister, opts.clientAuth)
	RegisterChatRoutes(mux, opts.chatRouter, opts.clientAuth)
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
