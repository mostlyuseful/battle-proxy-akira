package api

import (
	"encoding/json"
	"net/http"

	"battle-proxy-akira/internal/config"
	requestlog "battle-proxy-akira/internal/logging"
	"battle-proxy-akira/internal/router"
)

// Middleware wraps an HTTP handler. Client authentication can be supplied this way.
type Middleware func(http.Handler) http.Handler

// Option configures the API server.
type Option func(*serverOptions)

type serverOptions struct {
	modelLister     ModelLister
	chatRouter      router.Router
	responsesRouter router.Router
	clientAuth      Middleware
	requestLogger   requestlog.Logger
	maxBodyBytes    int64
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

// WithResponsesRouter configures the router used by POST /v1/responses.
// When unset, the Responses endpoint reuses the chat router.
func WithResponsesRouter(responsesRouter router.Router) Option {
	return func(opts *serverOptions) {
		opts.responsesRouter = responsesRouter
	}
}

// WithClientAuth configures middleware applied to client API routes.
func WithClientAuth(middleware Middleware) Option {
	return func(opts *serverOptions) {
		opts.clientAuth = middleware
	}
}

// WithRequestLogger configures metadata request logging for chat completions.
func WithRequestLogger(logger requestlog.Logger) Option {
	return func(opts *serverOptions) {
		opts.requestLogger = logger
	}
}

// WithServerConfig configures API behavior controlled by server config.
func WithServerConfig(cfg config.ServerConfig) Option {
	return func(opts *serverOptions) {
		opts.maxBodyBytes = cfg.MaxBodyBytes
	}
}

// NewServer builds the HTTP handler tree for the proxy API.
func NewServer(options ...Option) http.Handler {
	opts := serverOptions{
		modelLister:   ModelListerFunc(emptyModels),
		clientAuth:    identityMiddleware,
		requestLogger: requestlog.NoopLogger{},
		maxBodyBytes:  config.DefaultMaxBodyBytes,
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
	if opts.requestLogger == nil {
		opts.requestLogger = requestlog.NoopLogger{}
	}
	if opts.maxBodyBytes <= 0 {
		opts.maxBodyBytes = config.DefaultMaxBodyBytes
	}
	responsesRouter := opts.responsesRouter
	if responsesRouter == nil {
		responsesRouter = opts.chatRouter
	}

	mux := http.NewServeMux()
	RegisterHealthRoutes(mux)
	RegisterModelRoutes(mux, opts.modelLister, opts.clientAuth)
	RegisterChatRoutes(mux, opts.chatRouter, opts.clientAuth, opts.requestLogger, opts.maxBodyBytes)
	RegisterResponsesRoutes(mux, responsesRouter, opts.clientAuth, opts.requestLogger, opts.maxBodyBytes)
	return requestIDMiddleware(mux)
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
