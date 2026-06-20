package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"battle-proxy-akira/internal/ir"
	providerpkg "battle-proxy-akira/internal/provider"
	"battle-proxy-akira/internal/router"
)

// responsesFakeProvider is a configurable fake provider for Responses tests.
type responsesFakeProvider struct {
	name      string
	resp      *ir.Response
	err       error
	called    int
	lastReq   ir.Request
	lastModel string
}

func (p *responsesFakeProvider) Name() string { return p.name }

func (p *responsesFakeProvider) Complete(ctx context.Context, req ir.Request) (*ir.Response, error) {
	p.called++
	p.lastReq = req
	p.lastModel = req.Model
	if p.err != nil {
		return nil, p.err
	}
	if p.resp != nil {
		out := *p.resp
		out.Model = req.Model
		return &out, nil
	}
	return &ir.Response{
		ID:    "resp_fake",
		Model: req.Model,
		Message: ir.Message{
			Role:    ir.RoleAssistant,
			Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "hello from responses"}},
		},
		FinishReason: "stop",
		Usage:        &ir.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7},
	}, nil
}

func (p *responsesFakeProvider) Stream(context.Context, ir.Request) (<-chan ir.Event, error) {
	return nil, errors.New("stream not supported")
}
func (p *responsesFakeProvider) Models(context.Context) ([]ir.Model, error) { return nil, nil }
func (p *responsesFakeProvider) Health(context.Context) error               { return nil }

func newResponsesRouter(provider *responsesFakeProvider) router.Router {
	return &singleCandidateRouter{provider: provider}
}

// singleCandidateRouter returns one candidate wrapping a fake provider.
type singleCandidateRouter struct {
	provider *responsesFakeProvider
}

func (r *singleCandidateRouter) Resolve(ctx context.Context, req ir.Request) ([]router.RouteCandidate, error) {
	return []router.RouteCandidate{{
		ProviderName:   r.provider.name,
		ProviderModel:  req.Model,
		RequestedModel: req.Model,
		Provider:       r.provider,
	}}, nil
}

func (r *singleCandidateRouter) MarkFailure(router.RouteCandidate, error) {}
func (r *singleCandidateRouter) MarkSuccess(router.RouteCandidate)        {}

func TestResponsesNonStreamingTextEndToEnd(t *testing.T) {
	t.Parallel()

	provider := &responsesFakeProvider{name: "test_provider"}
	logger := &recordingLogger{}
	handler := NewServer(WithResponsesRouter(newResponsesRouter(provider)), WithRequestLogger(logger))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model": "gpt-test",
		"input": "write a test",
		"instructions": "be concise",
		"temperature": 0.2
	}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}
	if provider.called != 1 {
		t.Fatalf("provider called %d times, want 1", provider.called)
	}
	// The provider should receive the provider model, not the requested alias.
	if provider.lastModel != "gpt-test" {
		t.Fatalf("provider model = %q, want gpt-test", provider.lastModel)
	}
	// Instructions normalized to a developer message.
	if len(provider.lastReq.Messages) != 2 {
		t.Fatalf("IR messages len = %d, want 2", len(provider.lastReq.Messages))
	}
	if provider.lastReq.Messages[0].Role != "developer" {
		t.Fatalf("first message role = %q, want developer", provider.lastReq.Messages[0].Role)
	}
	if provider.lastReq.Messages[1].Role != "user" {
		t.Fatalf("second message role = %q, want user", provider.lastReq.Messages[1].Role)
	}

	var body struct {
		ID         string `json:"id"`
		Object     string `json:"object"`
		Model      string `json:"model"`
		Status     string `json:"status"`
		Output     []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Object != "response" {
		t.Fatalf("object = %q, want response", body.Object)
	}
	if body.Status != "completed" {
		t.Fatalf("status = %q, want completed", body.Status)
	}
	if len(body.Output) != 1 || body.Output[0].Type != "message" {
		t.Fatalf("output = %+v", body.Output)
	}
	if body.Output[0].Role != "assistant" {
		t.Fatalf("output role = %q", body.Output[0].Role)
	}
	if len(body.Output[0].Content) != 1 || body.Output[0].Content[0].Type != "output_text" {
		t.Fatalf("output content = %+v", body.Output[0].Content)
	}
	if body.Output[0].Content[0].Text != "hello from responses" {
		t.Fatalf("output text = %q", body.Output[0].Content[0].Text)
	}
	if body.Usage == nil || body.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %+v", body.Usage)
	}

	// Request metadata logged.
	if len(logger.records) != 1 {
		t.Fatalf("log records = %d, want 1", len(logger.records))
	}
	got := logger.records[0]
	if got.RequestedModel != "gpt-test" || got.ResolvedProvider != "test_provider" || got.Status != http.StatusOK {
		t.Fatalf("log record = %#v", got)
	}
}

func TestResponsesNonStreamingImageInput(t *testing.T) {
	t.Parallel()

	provider := &responsesFakeProvider{name: "test_provider"}
	logger := &recordingLogger{}
	handler := NewServer(WithResponsesRouter(newResponsesRouter(provider)), WithRequestLogger(logger))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model": "vision-model",
		"input": [
			{
				"type": "message",
				"role": "user",
				"content": [
					{"type": "input_text", "text": "describe this"},
					{"type": "input_image", "image_url": "data:image/png;base64,aW1hZ2UtYnl0ZXM=", "detail": "high"}
				]
			}
		]
	}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if provider.called != 1 {
		t.Fatalf("provider called %d times, want 1", provider.called)
	}
	if !provider.lastReq.HasImages() {
		t.Fatal("expected IR request to contain images")
	}
	// Image metadata logged.
	if len(logger.records) != 1 || len(logger.records[0].ImageInputs) != 1 {
		t.Fatalf("log records = %#v, want one image metadata entry", logger.records)
	}
}

func TestResponsesRewritesModelToRequestedAlias(t *testing.T) {
	t.Parallel()

	provider := &responsesFakeProvider{name: "test_provider"}
	// requestIDRouter sets RequestedModel = req.Model and candidate rewrites it back.
	handler := NewServer(WithResponsesRouter(newResponsesRouter(provider)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"coding","input":"hi"}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Model != "coding" {
		t.Fatalf("response model = %q, want requested alias coding", body.Model)
	}
}

func TestResponsesStreamingRejected(t *testing.T) {
	t.Parallel()

	provider := &responsesFakeProvider{name: "test_provider"}
	handler := NewServer(WithResponsesRouter(newResponsesRouter(provider)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi","stream":true}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var body OpenAIErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != string(ErrorInvalidRequest) {
		t.Fatalf("error code = %q, want %q", body.Error.Code, ErrorInvalidRequest)
	}
	if provider.called != 0 {
		t.Fatalf("provider should not be called for streaming request, called %d", provider.called)
	}
}

func TestResponsesErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		provider    *responsesFakeProvider
		wantStatus  int
		wantCode    ErrorCode
		wantNoCall  bool
	}{
		{
			name:       "bad json",
			body:       `{`,
			wantStatus: http.StatusBadRequest,
			wantCode:   ErrorInvalidRequest,
			wantNoCall: true,
		},
		{
			name:       "missing model",
			body:       `{"input":"hi"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   ErrorInvalidRequest,
			wantNoCall: true,
		},
		{
			name:        "upstream failure",
			body:        `{"model":"gpt-test","input":"hi"}`,
			provider:    &responsesFakeProvider{name: "test_provider", err: &providerpkg.Error{Code: providerpkg.ErrorUpstream, Retryable: false, Provider: "test_provider"}},
			wantStatus:  http.StatusBadGateway,
			wantCode:    ErrorUpstream,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			provider := tt.provider
			if provider == nil {
				provider = &responsesFakeProvider{name: "test_provider"}
			}
			handler := NewServer(WithResponsesRouter(newResponsesRouter(provider)))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(tt.body))
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status code = %d, want %d, body %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			var body OpenAIErrorResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if body.Error.Code != string(tt.wantCode) {
				t.Fatalf("error code = %q, want %q", body.Error.Code, tt.wantCode)
			}
			if tt.wantNoCall && provider.called != 0 {
				t.Fatalf("provider should not be called, called %d", provider.called)
			}
		})
	}
}

func TestResponsesNoRouterConfigured(t *testing.T) {
	t.Parallel()

	handler := NewServer()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi"}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	var body OpenAIErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != string(ErrorNoAvailableModel) {
		t.Fatalf("error code = %q, want %q", body.Error.Code, ErrorNoAvailableModel)
	}
}

func TestResponsesAppliesClientAuth(t *testing.T) {
	t.Parallel()

	provider := &responsesFakeProvider{name: "test_provider"}
	handler := NewServer(
		WithResponsesRouter(newResponsesRouter(provider)),
		WithClientAuth(StaticBearerAuth([]string{"client-token"})),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi"}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusForbidden)
	}
	var body OpenAIErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != string(ErrorPolicyDenied) {
		t.Fatalf("error code = %q, want %q", body.Error.Code, ErrorPolicyDenied)
	}
	if provider.called != 0 {
		t.Fatalf("provider should not be called for auth failure, called %d", provider.called)
	}
}

func TestResponsesRequestIDPropagated(t *testing.T) {
	t.Parallel()

	provider := &responsesFakeProvider{name: "test_provider"}
	handler := NewServer(WithResponsesRouter(newResponsesRouter(provider)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi"}`))
	req.Header.Set(requestIDHeader, "req_responses_123")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(requestIDHeader); got != "req_responses_123" {
		t.Fatalf("response request ID = %q", got)
	}
	if provider.lastReq.ID != "req_responses_123" {
		t.Fatalf("provider request ID = %q", provider.lastReq.ID)
	}
	if provider.lastReq.Metadata["request_id"] != "req_responses_123" {
		t.Fatalf("provider metadata request_id = %q", provider.lastReq.Metadata["request_id"])
	}
}

func TestResponsesRetriesRetryableProviderError(t *testing.T) {
	t.Parallel()

	// First provider fails retryably; second succeeds.
	failing := &responsesFakeProvider{name: "failing", err: &providerpkg.Error{Code: providerpkg.ErrorProviderRateLimited, Retryable: true, Provider: "failing"}}
	success := &responsesFakeProvider{name: "success"}
	rt := &multiCandidateRouter{providers: []*responsesFakeProvider{failing, success}}
	handler := NewServer(WithResponsesRouter(rt), WithRequestLogger(&recordingLogger{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi"}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, body %s", rec.Code, rec.Body.String())
	}
	if failing.called != 1 || success.called != 1 {
		t.Fatalf("failing called %d, success called %d, want 1/1", failing.called, success.called)
	}
}

// multiCandidateRouter returns multiple candidates in order for retry testing.
type multiCandidateRouter struct {
	providers []*responsesFakeProvider
}

func (r *multiCandidateRouter) Resolve(ctx context.Context, req ir.Request) ([]router.RouteCandidate, error) {
	candidates := make([]router.RouteCandidate, 0, len(r.providers))
	for _, p := range r.providers {
		candidates = append(candidates, router.RouteCandidate{
			ProviderName:   p.name,
			ProviderModel:  req.Model,
			RequestedModel: req.Model,
			Provider:       p,
		})
	}
	return candidates, nil
}

func (r *multiCandidateRouter) MarkFailure(router.RouteCandidate, error) {}
func (r *multiCandidateRouter) MarkSuccess(router.RouteCandidate)        {}
