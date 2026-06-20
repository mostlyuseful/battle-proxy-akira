package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"battle-proxy-akira/internal/config"
	requestlog "battle-proxy-akira/internal/logging"
	"battle-proxy-akira/internal/metrics"
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
	metrics         *metrics.Collector
	logger          *slog.Logger
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

// WithMetrics configures the runtime metrics collector. When set, the proxy
// records request counts, error counts, and latency summaries and exposes them
// at GET /metrics.
func WithMetrics(collector *metrics.Collector) Option {
	return func(opts *serverOptions) {
		opts.metrics = collector
	}
}

// WithLogger configures optional verbose diagnostics.
func WithLogger(logger *slog.Logger) Option {
	return func(opts *serverOptions) {
		opts.logger = logger
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
	RegisterMetricsRoutes(mux, opts.metrics)
	RegisterModelRoutes(mux, opts.modelLister, opts.clientAuth)
	RegisterChatRoutes(mux, opts.chatRouter, opts.clientAuth, opts.requestLogger, opts.maxBodyBytes)
	RegisterResponsesRoutes(mux, responsesRouter, opts.clientAuth, opts.requestLogger, opts.maxBodyBytes)
	handler := http.Handler(mux)
	if opts.metrics != nil {
		handler = metricsMiddleware(opts.metrics, handler)
	}
	if opts.logger != nil {
		opts.logger.Info("api routes registered", "routes", []string{"/healthz", "/readyz", "/metrics", "/v1/models", "/v1/chat/completions", "/v1/responses"})
		opts.logger.Info("api middleware configured", "client_auth", opts.clientAuth != nil, "request_logger", opts.requestLogger != requestlog.NoopLogger{}, "metrics", opts.metrics != nil, "max_body_bytes", opts.maxBodyBytes)
	}
	return requestIDMiddleware(handler)
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
