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

func newResponsesRouter(provider providerpkg.Provider) router.Router {
	return &singleCandidateRouter{provider: provider}
}

// singleCandidateRouter returns one candidate wrapping any provider.
type singleCandidateRouter struct {
	provider providerpkg.Provider
}

func (r *singleCandidateRouter) Resolve(ctx context.Context, req ir.Request) ([]router.RouteCandidate, error) {
	return []router.RouteCandidate{{
		ProviderName:   r.provider.Name(),
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

func TestResponsesStreamingEndToEnd(t *testing.T) {
	t.Parallel()

	provider := &responsesStreamProvider{
		name: "test_provider",
		chunks: []string{
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-test","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-test","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		},
		finishDone: true,
	}
	logger := &recordingLogger{}
	handler := NewServer(WithResponsesRouter(newResponsesRouter(provider)), WithRequestLogger(logger))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"coding","input":"hi","stream":true}`))
	req.Header.Set(requestIDHeader, "req_stream_1")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", got)
	}
	if !rec.Flushed {
		t.Fatal("response was not flushed")
	}

	body := rec.Body.String()
	wantEvents := []string{
		"event: response.created",
		"event: response.output_item.added",
		"event: response.content_part.added",
		"event: response.output_text.delta",
		"event: response.output_text.done",
		"event: response.content_part.done",
		"event: response.output_item.done",
		"event: response.completed",
	}
	for _, want := range wantEvents {
		if !strings.Contains(body, want) {
			t.Fatalf("stream body missing %q\n%s", want, body)
		}
	}
	// Deltas for both content chunks.
	if got := strings.Count(body, "event: response.output_text.delta"); got != 2 {
		t.Fatalf("output_text.delta count = %d, want 2", got)
	}
	if !strings.Contains(body, `"delta":"hello"`) || !strings.Contains(body, `"delta":" world"`) {
		t.Fatalf("stream body missing delta payloads\n%s", body)
	}
	// Full text in done/completed events.
	if !strings.Contains(body, `"text":"hello world"`) {
		t.Fatalf("stream body missing aggregated text\n%s", body)
	}
	// Response ID derived from request ID, requested model alias preserved.
	if !strings.Contains(body, `"id":"resp_stream_1"`) {
		t.Fatalf("stream body missing response id\n%s", body)
	}
	if !strings.Contains(body, `"model":"coding"`) {
		t.Fatalf("stream body missing requested model alias\n%s", body)
	}
	// Status completed on the final response object.
	if !strings.Contains(body, `"status":"completed"`) {
		t.Fatalf("stream body missing completed status\n%s", body)
	}

	if len(logger.records) != 1 {
		t.Fatalf("log records = %d, want 1", len(logger.records))
	}
	got := logger.records[0]
	if !got.Stream || got.Status != http.StatusOK || got.RequestedModel != "coding" || got.ResolvedProvider != "test_provider" {
		t.Fatalf("log record = %#v", got)
	}
}

func TestResponsesStreamingPreStreamErrorReturnsJSON(t *testing.T) {
	t.Parallel()

	provider := &responsesStreamProvider{
		name: "test_provider",
		streamErr: &providerpkg.Error{Code: providerpkg.ErrorProviderRateLimited, Retryable: false, Provider: "test_provider"},
	}
	handler := NewServer(WithResponsesRouter(newResponsesRouter(provider)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi","stream":true}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json for pre-stream error", got)
	}
	var body OpenAIErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != string(ErrorProviderRateLimited) {
		t.Fatalf("error code = %q, want %q", body.Error.Code, ErrorProviderRateLimited)
	}
}

func TestResponsesStreamingPreStreamRetryableFallsBack(t *testing.T) {
	t.Parallel()

	failing := &responsesStreamProvider{
		name: "failing",
		streamErr: &providerpkg.Error{Code: providerpkg.ErrorProviderRateLimited, Retryable: true, Provider: "failing"},
	}
	success := &responsesStreamProvider{
		name: "success",
		chunks: []string{
			`{"choices":[{"index":0,"delta":{"content":"ok"}}]}`,
		},
		finishDone: true,
	}
	rt := &multiCandidateStreamRouter{providers: []*responsesStreamProvider{failing, success}}
	handler := NewServer(WithResponsesRouter(rt))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi","stream":true}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"delta":"ok"`) {
		t.Fatalf("expected fallback stream to emit ok delta\n%s", rec.Body.String())
	}
}

func TestResponsesStreamingMidStreamErrorEmitsErrorEvent(t *testing.T) {
	t.Parallel()

	provider := &responsesStreamProvider{
		name: "test_provider",
		chunks: []string{
			`{"choices":[{"index":0,"delta":{"content":"partial"}}]}`,
		},
		midStreamErr: &providerpkg.Error{Code: providerpkg.ErrorUpstream, Provider: "test_provider"},
	}
	handler := NewServer(WithResponsesRouter(newResponsesRouter(provider)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi","stream":true}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d (SSE already started)", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: response.output_text.delta") {
		t.Fatalf("expected delta before mid-stream error\n%s", body)
	}
	if !strings.Contains(body, "event: error") {
		t.Fatalf("expected error SSE event on mid-stream failure\n%s", body)
	}
	if !strings.Contains(body, `"code":"upstream_error"`) {
		t.Fatalf("expected upstream_error code in error event\n%s", body)
	}
	// Closing events must not be emitted after a mid-stream error.
	if strings.Contains(body, "event: response.completed") {
		t.Fatalf("response.completed must not be emitted after mid-stream error\n%s", body)
	}
}

func TestResponsesStreamingEmptyStreamStillEmitsLifecycle(t *testing.T) {
	t.Parallel()

	provider := &responsesStreamProvider{name: "test_provider", finishDone: true}
	handler := NewServer(WithResponsesRouter(newResponsesRouter(provider)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi","stream":true}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"event: response.created", "event: response.completed"} {
		if !strings.Contains(body, want) {
			t.Fatalf("empty stream missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "event: response.output_text.delta") {
		t.Fatalf("empty stream should not emit output_text.delta\n%s", body)
	}
}

func TestResponsesStreamingCarriesUsageFromFinalChunk(t *testing.T) {
	t.Parallel()

	provider := &responsesStreamProvider{
		name: "test_provider",
		chunks: []string{
			`{"choices":[{"index":0,"delta":{"content":"hi"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`,
		},
		finishDone: true,
	}
	handler := NewServer(WithResponsesRouter(newResponsesRouter(provider)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi","stream":true}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"input_tokens":3`) || !strings.Contains(body, `"total_tokens":7`) {
		t.Fatalf("expected usage in completed event\n%s", body)
	}
}

func TestResponsesStreamingEndToEndWithRealProvider(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "upstream-token")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("upstream accept = %q, want text/event-stream", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"}}]}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer upstream.Close()

	handler := NewServer(WithResponsesRouter(newTestChatRouter(t, upstream.URL)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"hi","stream":true}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type = %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: response.created",
		"event: response.output_text.delta",
		"event: response.completed",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q\n%s", want, body)
		}
	}
	if !strings.Contains(body, `"delta":"hello"`) || !strings.Contains(body, `"delta":" world"`) {
		t.Fatalf("missing delta payloads translated from upstream chunks\n%s", body)
	}
	if !strings.Contains(body, `"text":"hello world"`) {
		t.Fatalf("missing aggregated text\n%s", body)
	}
	// No raw Chat Completions chunks should leak through.
	if strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("raw chat completion chunk leaked into Responses stream\n%s", body)
	}
	// No [DONE] marker (that is Chat Completions-specific).
	if strings.Contains(body, "[DONE]") {
		t.Fatalf("[DONE] marker should not appear in Responses stream\n%s", body)
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

// responsesStreamProvider is a fake provider that emits Chat Completions-shaped
// streaming chunks, matching the real OpenAI-compatible provider's ir.Event shape.
type responsesStreamProvider struct {
	name        string
	chunks      []string
	finishDone  bool
	streamErr   error
	midStreamErr error
	streamed    int
}

func (p *responsesStreamProvider) Name() string { return p.name }

func (p *responsesStreamProvider) Complete(context.Context, ir.Request) (*ir.Response, error) {
	return nil, errors.New("complete not supported")
}

func (p *responsesStreamProvider) Stream(ctx context.Context, req ir.Request) (<-chan ir.Event, error) {
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	events := make(chan ir.Event, len(p.chunks)+1)
	go func() {
		defer close(events)
		for i, chunk := range p.chunks {
			events <- ir.Event{
				Type:  ir.EventTypeMessageDelta,
				Model: req.Model,
				Text:  chunk,
				Raw:   json.RawMessage(chunk),
			}
			// After the first real chunk, optionally surface a mid-stream failure.
			if i == 0 && p.midStreamErr != nil {
				events <- ir.Event{Type: ir.EventTypeError, Model: req.Model, Error: &ir.Error{Code: providerpkg.ErrorCode(p.midStreamErr), Message: "upstream failed"}}
				return
			}
		}
		if p.finishDone {
			events <- ir.Event{Type: ir.EventTypeDone, Model: req.Model, Text: "[DONE]"}
		}
	}()
	p.streamed++
	return events, nil
}

func (p *responsesStreamProvider) Models(context.Context) ([]ir.Model, error) { return nil, nil }
func (p *responsesStreamProvider) Health(context.Context) error               { return nil }

// multiCandidateStreamRouter returns multiple stream-provider candidates in order.
type multiCandidateStreamRouter struct {
	providers []*responsesStreamProvider
}

func (r *multiCandidateStreamRouter) Resolve(ctx context.Context, req ir.Request) ([]router.RouteCandidate, error) {
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

func (r *multiCandidateStreamRouter) MarkFailure(router.RouteCandidate, error) {}
func (r *multiCandidateStreamRouter) MarkSuccess(router.RouteCandidate)        {}
