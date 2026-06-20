package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"battle-proxy-akira/internal/metrics"
)

// metricsCollectorContextKey carries the active metrics collector through a
// request so handlers can record internal error codes alongside the automatic
// endpoint/status/latency recording done by metricsMiddleware.
type metricsCollectorContextKey struct{}

// ContextWithMetricsCollector returns a context carrying the collector.
func ContextWithMetricsCollector(ctx context.Context, collector *metrics.Collector) context.Context {
	if collector == nil {
		return ctx
	}
	return context.WithValue(ctx, metricsCollectorContextKey{}, collector)
}

// MetricsCollectorFromContext returns the collector carried by ctx, if any.
func MetricsCollectorFromContext(ctx context.Context) *metrics.Collector {
	c, _ := ctx.Value(metricsCollectorContextKey{}).(*metrics.Collector)
	return c
}

// RegisterMetricsRoutes wires the structured metrics endpoint. When collector
// is nil the endpoint reports an empty snapshot.
func RegisterMetricsRoutes(mux *http.ServeMux, collector *metrics.Collector) {
	mux.Handle("GET /metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var snap metrics.Snapshot
		if collector != nil {
			snap = collector.Snapshot()
		}
		writeJSON(w, http.StatusOK, snap)
	}))
}

// statusRecorder captures the response status code for metrics.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

// metricsMiddleware records request counts by endpoint/status-class and latency
// for every request. It is installed outside the request-ID middleware so the
// collector is available in the request context for error-code recording.
func metricsMiddleware(collector *metrics.Collector, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		ctx := ContextWithMetricsCollector(r.Context(), collector)
		next.ServeHTTP(rec, r.WithContext(ctx))

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		collector.RecordRequest(metrics.EndpointForPath(r.URL.Path), metrics.StatusClass(status), time.Since(started))
	})
}

// recordErrorFromProxy increments the error-code counter when a collector is
// present in the context. Unknown/empty codes are ignored.
func recordErrorFromProxy(ctx context.Context, code ErrorCode) {
	if code == "" {
		return
	}
	if c := MetricsCollectorFromContext(ctx); c != nil {
		c.RecordError(string(code))
	}
}

// metricsSnapshotJSON encodes a snapshot for testing convenience.
func metricsSnapshotJSON(snap metrics.Snapshot) ([]byte, error) {
	return json.Marshal(snap)
}
