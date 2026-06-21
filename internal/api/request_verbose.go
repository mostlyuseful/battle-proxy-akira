package api

import (
	"log/slog"
	"net/http"

	requestlog "battle-proxy-akira/internal/logging"
)

func logRequestAccepted(logger *slog.Logger, r *http.Request, requestID, endpoint string) {
	if logger == nil || r == nil {
		return
	}
	logger.Info("request accepted", "request_id", requestID, "endpoint", endpoint, "method", r.Method, "path", r.URL.Path, "stream", r.URL.Query().Get("stream") == "true")
}

func logRequestStarted(logger *slog.Logger, rec requestlog.RequestLogRecord) {
	if logger == nil {
		return
	}
	attrs := []any{"request_id", rec.RequestID, "endpoint", rec.Endpoint, "stream", rec.Stream}
	if rec.RequestedModel != "" {
		attrs = append(attrs, "requested_model", rec.RequestedModel)
	}
	logger.Info("request started", attrs...)
}

func logRequestFinished(logger *slog.Logger, rec requestlog.RequestLogRecord) {
	if logger == nil {
		return
	}
	attrs := []any{
		"request_id", rec.RequestID,
		"endpoint", rec.Endpoint,
		"status", rec.Status,
		"latency_ms", rec.LatencyMS,
		"stream", rec.Stream,
		"retry_count", rec.RetryCount,
	}
	if rec.RequestedModel != "" {
		attrs = append(attrs, "requested_model", rec.RequestedModel)
	}
	if rec.ResolvedProvider != "" {
		attrs = append(attrs, "resolved_provider", rec.ResolvedProvider)
	}
	if rec.ResolvedModel != "" {
		attrs = append(attrs, "resolved_model", rec.ResolvedModel)
	}
	logger.Info("request finished", attrs...)
}
