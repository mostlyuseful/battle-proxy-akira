package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"battle-proxy-akira/internal/ir"
	"battle-proxy-akira/internal/router"
)

func TestRequestIDMiddlewareGeneratesAndPreservesSafeIDs(t *testing.T) {
	t.Parallel()

	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(RequestIDFromContext(r.Context())))
	}))

	generatedRec := httptest.NewRecorder()
	handler.ServeHTTP(generatedRec, httptest.NewRequest(http.MethodGet, "/", nil))
	generatedID := generatedRec.Header().Get(requestIDHeader)
	if !strings.HasPrefix(generatedID, "req_") || generatedRec.Body.String() != generatedID {
		t.Fatalf("generated request ID header/body = %q/%q", generatedID, generatedRec.Body.String())
	}

	preservedRec := httptest.NewRecorder()
	preservedReq := httptest.NewRequest(http.MethodGet, "/", nil)
	preservedReq.Header.Set(requestIDHeader, "client.req-123:abc")
	handler.ServeHTTP(preservedRec, preservedReq)
	if got := preservedRec.Header().Get(requestIDHeader); got != "client.req-123:abc" || preservedRec.Body.String() != got {
		t.Fatalf("preserved request ID header/body = %q/%q", got, preservedRec.Body.String())
	}

	unsafeRec := httptest.NewRecorder()
	unsafeReq := httptest.NewRequest(http.MethodGet, "/", nil)
	unsafeReq.Header.Set(requestIDHeader, "bad id with spaces")
	handler.ServeHTTP(unsafeRec, unsafeReq)
	if got := unsafeRec.Header().Get(requestIDHeader); got == "bad id with spaces" || !strings.HasPrefix(got, "req_") {
		t.Fatalf("unsafe request ID was not replaced: %q", got)
	}
}

func TestChatCompletionsRequestIDPropagatesToContextProviderLogAndHeader(t *testing.T) {
	logger := &recordingLogger{}
	provider := &requestIDProvider{}
	router := &requestIDRouter{provider: provider}
	handler := NewServer(WithChatRouter(router), WithRequestLogger(logger))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set(requestIDHeader, "req_client_123")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get(requestIDHeader); got != "req_client_123" {
		t.Fatalf("response request ID = %q, want req_client_123", got)
	}
	if router.resolveContextID != "req_client_123" || router.resolveRequestID != "req_client_123" || router.resolveMetadataID != "req_client_123" {
		t.Fatalf("router IDs = context %q request %q metadata %q", router.resolveContextID, router.resolveRequestID, router.resolveMetadataID)
	}
	if provider.contextID != "req_client_123" || provider.requestID != "req_client_123" || provider.metadataID != "req_client_123" {
		t.Fatalf("provider IDs = context %q request %q metadata %q", provider.contextID, provider.requestID, provider.metadataID)
	}
	if len(logger.records) != 1 || logger.records[0].RequestID != "req_client_123" {
		t.Fatalf("log records = %#v, want request ID req_client_123", logger.records)
	}
}

func TestChatCompletionsErrorIncludesRequestIDHeaderAndLog(t *testing.T) {
	logger := &recordingLogger{}
	handler := NewServer(WithRequestLogger(logger))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set(requestIDHeader, "req_error_123")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get(requestIDHeader); got != "req_error_123" {
		t.Fatalf("error response request ID = %q, want req_error_123", got)
	}
	if len(logger.records) != 1 || logger.records[0].RequestID != "req_error_123" || logger.records[0].Status != http.StatusServiceUnavailable {
		t.Fatalf("error log records = %#v", logger.records)
	}
}

type requestIDRouter struct {
	provider          *requestIDProvider
	resolveContextID  string
	resolveRequestID  string
	resolveMetadataID string
}

func (r *requestIDRouter) Resolve(ctx context.Context, req ir.Request) ([]router.RouteCandidate, error) {
	r.resolveContextID = RequestIDFromContext(ctx)
	r.resolveRequestID = req.ID
	r.resolveMetadataID = req.Metadata["request_id"]
	return []router.RouteCandidate{{
		ProviderName:   "test_provider",
		ProviderModel:  "gpt-test",
		RequestedModel: req.Model,
		Provider:       r.provider,
	}}, nil
}

func (r *requestIDRouter) MarkFailure(router.RouteCandidate, error) {}
func (r *requestIDRouter) MarkSuccess(router.RouteCandidate)        {}

type requestIDProvider struct {
	contextID  string
	requestID  string
	metadataID string
}

func (p *requestIDProvider) Name() string { return "test_provider" }

func (p *requestIDProvider) Complete(ctx context.Context, req ir.Request) (*ir.Response, error) {
	p.contextID = RequestIDFromContext(ctx)
	p.requestID = req.ID
	p.metadataID = req.Metadata["request_id"]
	return &ir.Response{
		ID:    "chatcmpl-test",
		Model: req.Model,
		Message: ir.Message{
			Role:    ir.RoleAssistant,
			Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "ok"}},
		},
		FinishReason: "stop",
	}, nil
}

func (p *requestIDProvider) Stream(context.Context, ir.Request) (<-chan ir.Event, error) {
	return nil, nil
}

func (p *requestIDProvider) Models(context.Context) ([]ir.Model, error) { return nil, nil }
func (p *requestIDProvider) Health(context.Context) error               { return nil }
