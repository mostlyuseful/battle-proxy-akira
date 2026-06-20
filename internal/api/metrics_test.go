package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"battle-proxy-akira/internal/ir"
	"battle-proxy-akira/internal/metrics"
)

func TestMetricsEndpointReturnsJSONShape(t *testing.T) {
	t.Parallel()

	collector := metrics.NewCollector()
	collector.RecordRequest(metrics.EndpointChatCompletions, "2xx", 5*1_000_000)
	collector.RecordError("upstream_error")
	handler := NewServer(WithMetrics(collector))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q", got)
	}

	var snap metrics.Snapshot
	if err := json.NewDecoder(rec.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.TotalRequests != 1 {
		t.Fatalf("TotalRequests = %d, want 1", snap.TotalRequests)
	}
	if snap.TotalErrors != 1 {
		t.Fatalf("TotalErrors = %d, want 1", snap.TotalErrors)
	}
	if len(snap.Requests) != 1 || snap.Requests[0].Endpoint != metrics.EndpointChatCompletions {
		t.Fatalf("Requests = %+v", snap.Requests)
	}
	if len(snap.Errors) != 1 || snap.Errors[0].Code != "upstream_error" {
		t.Fatalf("Errors = %+v", snap.Errors)
	}
	if len(snap.Latency) != 1 || snap.Latency[0].MeanMS != 5 {
		t.Fatalf("Latency = %+v", snap.Latency)
	}
}

func TestMetricsEndpointWithNilCollectorReturnsEmpty(t *testing.T) {
	t.Parallel()

	handler := NewServer()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var snap metrics.Snapshot
	if err := json.NewDecoder(rec.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.TotalRequests != 0 || snap.TotalErrors != 0 {
		t.Fatalf("expected empty snapshot, got %+v", snap)
	}
}

func TestMetricsMiddlewareRecordsEndpointStatusAndLatency(t *testing.T) {
	t.Parallel()

	collector := metrics.NewCollector()
	handler := NewServer(
		WithMetrics(collector),
		WithResponsesRouter(&singleCandidateRouter{provider: &responsesFakeProvider{name: "test_provider"}}),
	)

	// Successful Responses request.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi"}`))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	// Failed Responses request (missing model).
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"hi"}`))
	handler.ServeHTTP(rec2, req2)

	snap := collector.Snapshot()
	if snap.TotalRequests != 2 {
		t.Fatalf("TotalRequests = %d, want 2", snap.TotalRequests)
	}
	// One 2xx (success) and one 4xx (invalid_request) for the responses endpoint.
	count := func(class string) int64 {
		for _, m := range snap.Requests {
			if m.Endpoint == metrics.EndpointResponses && m.StatusClass == class {
				return m.Count
			}
		}
		return 0
	}
	if got := count("2xx"); got != 1 {
		t.Fatalf("responses 2xx = %d, want 1", got)
	}
	if got := count("4xx"); got != 1 {
		t.Fatalf("responses 4xx = %d, want 1", got)
	}
	// Latency bucket present for the responses 2xx path.
	var hasLatency bool
	for _, m := range snap.Latency {
		if m.Endpoint == metrics.EndpointResponses && m.StatusClass == "2xx" && m.Count == 1 {
			hasLatency = true
		}
	}
	if !hasLatency {
		t.Fatalf("expected latency bucket for responses 2xx, got %+v", snap.Latency)
	}
}

func TestMetricsMiddlewareRecordsErrorCodes(t *testing.T) {
	t.Parallel()

	collector := metrics.NewCollector()
	// No router configured -> no_available_model error path.
	handler := NewServer(WithMetrics(collector))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`))
	handler.ServeHTTP(rec, req)

	snap := collector.Snapshot()
	if snap.TotalErrors != 1 {
		t.Fatalf("TotalErrors = %d, want 1", snap.TotalErrors)
	}
	if len(snap.Errors) != 1 || snap.Errors[0].Code != "no_available_model" {
		t.Fatalf("Errors = %+v, want no_available_model", snap.Errors)
	}
	// The error request is also counted as a 5xx request.
	count := func(class string) int64 {
		for _, m := range snap.Requests {
			if m.Endpoint == metrics.EndpointChatCompletions && m.StatusClass == class {
				return m.Count
			}
		}
		return 0
	}
	if got := count("5xx"); got != 1 {
		t.Fatalf("chat 5xx = %d, want 1", got)
	}
}

func TestMetricsMiddlewareRecordsHealthEndpoint(t *testing.T) {
	t.Parallel()

	collector := metrics.NewCollector()
	handler := NewServer(WithMetrics(collector))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	snap := collector.Snapshot()
	if snap.TotalRequests != 1 {
		t.Fatalf("TotalRequests = %d, want 1", snap.TotalRequests)
	}
	if snap.Requests[0].Endpoint != metrics.EndpointHealth || snap.Requests[0].StatusClass != "2xx" {
		t.Fatalf("Requests = %+v", snap.Requests)
	}
}

// Ensure the metricsCollectorContextKey plumbing is exercised: error codes are
// recorded via the context-stored collector inside writeLoggedOpenAIError.
func TestMetricsCollectorFromContext(t *testing.T) {
	t.Parallel()

	collector := metrics.NewCollector()
	ctx := ContextWithMetricsCollector(context.Background(), collector)
	if got := MetricsCollectorFromContext(ctx); got != collector {
		t.Fatal("collector not retrieved from context")
	}
	recordErrorFromProxy(ctx, "upstream_error")
	if got := collector.Snapshot().TotalErrors; got != 1 {
		t.Fatalf("TotalErrors = %d, want 1", got)
	}
}

// Sanity: nil context is a no-op for error recording.
func TestMetricsNilCollectorContextIsNoOp(t *testing.T) {
	t.Parallel()

	c := MetricsCollectorFromContext(context.Background())
	if c != nil {
		t.Fatalf("expected nil collector from empty context, got %v", c)
	}
	recordErrorFromProxy(context.Background(), "upstream_error") // must not panic
}

// Sanity: the endpoint label mapping is reflected in recorded metrics.
func TestMetricsEndpointLabelForChatCompletions(t *testing.T) {
	t.Parallel()

	collector := metrics.NewCollector()
	handler := NewServer(
		WithMetrics(collector),
		WithChatRouter(&requestIDRouter{provider: &requestIDProvider{}}),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	snap := collector.Snapshot()
	if snap.TotalRequests != 1 || snap.Requests[0].Endpoint != metrics.EndpointChatCompletions {
		t.Fatalf("unexpected metrics: %+v", snap)
	}
}

// reference ir to keep the import meaningful for the fake provider router.
var _ = ir.ModalityText
