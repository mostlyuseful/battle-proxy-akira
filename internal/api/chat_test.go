package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"battle-proxy-akira/internal/auth"
	"battle-proxy-akira/internal/config"
	"battle-proxy-akira/internal/ir"
	requestlog "battle-proxy-akira/internal/logging"
	providerpkg "battle-proxy-akira/internal/provider"
	"battle-proxy-akira/internal/router"
)

func TestChatCompletionsNonStreamingEndToEnd(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "upstream-token")

	var captured struct {
		Path          string
		Authorization string
		Body          map[string]any
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Path = r.URL.Path
		captured.Authorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&captured.Body); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-upstream",
			"object": "chat.completion",
			"created": 123,
			"model": "gpt-test",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "hello from upstream"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7}
		}`))
	}))
	defer upstream.Close()

	handler := NewServer(WithChatRouter(newTestChatRouter(t, upstream.URL)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model": "gpt-test",
		"messages": [{"role": "user", "content": "hello"}],
		"temperature": 0.2
	}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}
	if captured.Path != "/v1/chat/completions" {
		t.Fatalf("upstream path = %q, want /v1/chat/completions", captured.Path)
	}
	if captured.Authorization != "Bearer upstream-token" {
		t.Fatalf("upstream authorization = %q, want bearer token", captured.Authorization)
	}
	if captured.Body["model"] != "gpt-test" {
		t.Fatalf("upstream model = %#v, want gpt-test", captured.Body["model"])
	}
	if captured.Body["stream"] != nil {
		t.Fatalf("upstream stream = %#v, want absent/false for non-stream path", captured.Body["stream"])
	}

	var body struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ID != "chatcmpl-upstream" || body.Object != "chat.completion" || body.Model != "gpt-test" {
		t.Fatalf("response metadata = %#v", body)
	}
	if body.Choices[0].Message.Role != "assistant" || body.Choices[0].Message.Content != "hello from upstream" {
		t.Fatalf("choice message = %#v", body.Choices[0].Message)
	}
	if body.Choices[0].FinishReason != "stop" || body.Usage.TotalTokens != 7 {
		t.Fatalf("finish/usage = %#v", body)
	}
}

func TestChatCompletionsStreamingEndToEnd(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "upstream-token")

	var captured struct {
		Path          string
		Authorization string
		Accept        string
		Body          map[string]any
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Path = r.URL.Path
		captured.Authorization = r.Header.Get("Authorization")
		captured.Accept = r.Header.Get("Accept")
		if err := json.NewDecoder(r.Body).Decode(&captured.Body); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer upstream.Close()

	handler := NewServer(WithChatRouter(newTestChatRouter(t, upstream.URL)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model": "gpt-test",
		"messages": [{"role": "user", "content": "hello"}],
		"stream": true
	}`))
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
	if captured.Path != "/v1/chat/completions" {
		t.Fatalf("upstream path = %q, want /v1/chat/completions", captured.Path)
	}
	if captured.Authorization != "Bearer upstream-token" {
		t.Fatalf("upstream authorization = %q, want bearer token", captured.Authorization)
	}
	if captured.Accept != "text/event-stream" {
		t.Fatalf("upstream accept = %q, want text/event-stream", captured.Accept)
	}
	if captured.Body["stream"] != true {
		t.Fatalf("upstream stream = %#v, want true", captured.Body["stream"])
	}

	want := "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n" +
		"data: [DONE]\n\n"
	if rec.Body.String() != want {
		t.Fatalf("stream body = %q, want %q", rec.Body.String(), want)
	}
}

func TestChatCompletionsStreamingPreStreamError(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "upstream-token")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failed", http.StatusTooManyRequests)
	}))
	defer upstream.Close()

	handler := NewServer(WithChatRouter(newTestChatRouter(t, upstream.URL)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}],"stream":true}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}
	var body OpenAIErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != string(ErrorProviderRateLimited) {
		t.Fatalf("error code = %q, want %q", body.Error.Code, ErrorProviderRateLimited)
	}
}

func TestChatCompletionsErrors(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "upstream-token")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failed", http.StatusBadGateway)
	}))
	defer upstream.Close()

	handler := NewServer(WithChatRouter(newTestChatRouter(t, upstream.URL)))

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   ErrorCode
	}{
		{name: "bad json", body: `{`, wantStatus: http.StatusBadRequest, wantCode: ErrorInvalidRequest},
		{name: "unknown model", body: `{"model":"missing","messages":[{"role":"user","content":"hello"}]}`, wantStatus: http.StatusNotFound, wantCode: ErrorUnknownModel},
		{name: "upstream failure", body: `{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`, wantStatus: http.StatusBadGateway, wantCode: ErrorUpstream},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tt.body))
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
		})
	}
}

func TestChatCompletionsImageRequestUnsupportedModality(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "upstream-token")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called for unsupported modality")
	}))
	defer upstream.Close()

	handler := NewServer(WithChatRouter(newTestChatRouter(t, upstream.URL)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"gpt-test",
		"messages":[{"role":"user","content":[
			{"type":"text","text":"describe"},
			{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}
		]}]
	}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
	var body OpenAIErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != string(ErrorUnsupportedModality) {
		t.Fatalf("error code = %q, want %q", body.Error.Code, ErrorUnsupportedModality)
	}
}

func TestChatCompletionsFailureLogDoesNotContainBearerOrAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-upstream-secret-token")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "sk-upstream-secret-token client-secret-token", http.StatusBadGateway)
	}))
	defer upstream.Close()

	logPath := filepath.Join(t.TempDir(), "requests.jsonl")
	logger, err := requestlog.New(config.LoggingConfig{Enabled: true, Mode: config.LoggingModeMetadataOnly, Path: logPath})
	if err != nil {
		t.Fatalf("New logger: %v", err)
	}
	handler := NewServer(
		WithChatRouter(newTestChatRouter(t, upstream.URL)),
		WithClientAuth(StaticBearerAuth([]string{"client-secret-token"})),
		WithRequestLogger(logger),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Authorization", "Bearer client-secret-token")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	combined := rec.Body.String()
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	combined += string(logData)
	for _, secret := range []string{"sk-upstream-secret-token", "client-secret-token"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("response/log leaked %q in %s", secret, combined)
		}
	}
}

func TestChatCompletionsMetadataLogging(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "upstream-token")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-upstream",
			"object": "chat.completion",
			"created": 123,
			"model": "gpt-test",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}]
		}`))
	}))
	defer upstream.Close()

	logger := &recordingLogger{}
	handler := NewServer(
		WithChatRouter(newTestChatRouter(t, upstream.URL)),
		WithRequestLogger(logger),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("X-Request-ID", "req_known")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if len(logger.records) != 1 {
		t.Fatalf("log records length = %d, want 1", len(logger.records))
	}
	got := logger.records[0]
	if got.RequestID != "req_known" || got.RequestedModel != "gpt-test" || got.ResolvedProvider != "openai_api" || got.ResolvedModel != "gpt-test" {
		t.Fatalf("log routing metadata = %#v", got)
	}
	if got.Stream || got.Status != http.StatusOK || got.RetryCount != 0 || got.LatencyMS < 0 {
		t.Fatalf("log status metadata = %#v", got)
	}
}

func TestChatCompletionsLogsImageMetadata(t *testing.T) {
	logger := &recordingLogger{}
	handler := NewServer(
		WithChatRouter(&requestIDRouter{provider: &requestIDProvider{}}),
		WithRequestLogger(logger),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"gpt-test",
		"messages":[{"role":"user","content":[
			{"type":"text","text":"describe"},
			{"type":"image_url","image_url":{"url":"data:image/png;base64,aW1hZ2UtYnl0ZXM="}}
		]}]
	}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(logger.records) != 1 || len(logger.records[0].ImageInputs) != 1 {
		t.Fatalf("log records = %#v, want one image metadata entry", logger.records)
	}
	image := logger.records[0].ImageInputs[0]
	if image.MIMEType != "image/png" || image.ByteLength != len("image-bytes") || image.SHA256 == "" {
		t.Fatalf("image metadata = %#v", image)
	}
}

func TestChatCompletionsInvasiveLoggingCapturesSessionAndTranscript(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "upstream-token")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-upstream",
			"object": "chat.completion",
			"created": 123,
			"model": "gpt-test",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}]
		}`))
	}))
	defer upstream.Close()

	logger := &recordingLogger{mode: config.LoggingModeInvasive}
	handler := NewServer(
		WithChatRouter(newTestChatRouter(t, upstream.URL)),
		WithRequestLogger(logger),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hello sk-secret-token"}]}`))
	req.Header.Set(sessionIDHeader, "sess_123")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if len(logger.records) != 1 {
		t.Fatalf("log records length = %d, want 1", len(logger.records))
	}
	got := logger.records[0]
	if got.SessionID != "sess_123" {
		t.Fatalf("session_id = %q, want sess_123", got.SessionID)
	}
	transcript, ok := got.Transcript.(*requestlog.Transcript)
	if !ok || transcript == nil || len(transcript.Attempts) != 1 || len(transcript.Request) == 0 || len(transcript.Attempts[0].Response) == 0 {
		t.Fatalf("transcript = %#v", got.Transcript)
	}
}

func TestChatCompletionsLoggingFailureDoesNotBreakSuccess(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "upstream-token")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-upstream",
			"object": "chat.completion",
			"created": 123,
			"model": "gpt-test",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}]
		}`))
	}))
	defer upstream.Close()

	handler := NewServer(
		WithChatRouter(newTestChatRouter(t, upstream.URL)),
		WithRequestLogger(failingLogger{}),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestChatCompletionsAppliesClientAuth(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "upstream-token")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called for auth failure")
	}))
	defer upstream.Close()

	handler := NewServer(
		WithChatRouter(newTestChatRouter(t, upstream.URL)),
		WithClientAuth(StaticBearerAuth([]string{"client-token"})),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusForbidden)
	}
	var body OpenAIErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != string(ErrorPolicyDenied) {
		t.Fatalf("error code = %q, want %q", body.Error.Code, ErrorPolicyDenied)
	}
}

type recordingLogger struct {
	records []requestlog.RequestLogRecord
	mode    string
}

func (l *recordingLogger) Mode() string {
	if l == nil || l.mode == "" {
		return config.LoggingModeMetadataOnly
	}
	return l.mode
}

func (l *recordingLogger) LogRequest(ctx context.Context, rec requestlog.RequestLogRecord) error {
	l.records = append(l.records, rec)
	return nil
}

type failingLogger struct{}

func (failingLogger) LogRequest(context.Context, requestlog.RequestLogRecord) error {
	return errors.New("log failed")
}

func newTestChatRouter(t *testing.T, upstreamURL string) router.Router {
	t.Helper()

	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"openai_api": {
			Type:    config.ProviderTypeOpenAICompatible,
			BaseURL: upstreamURL + "/v1",
			Auth: config.AuthConfig{
				Type: config.AuthTypeBearerEnv,
				Env:  "OPENAI_API_KEY",
			},
			Models: map[string]config.ModelConfig{
				"gpt-test": {Modalities: []string{ir.ModalityText}},
			},
		},
	}
	tokenSource, err := auth.NewTokenSource(cfg.Providers["openai_api"].Auth)
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}
	upstreamProvider, err := providerpkg.NewOpenAICompatible("openai_api", cfg.Providers["openai_api"], tokenSource, http.DefaultClient)
	if err != nil {
		t.Fatalf("NewOpenAICompatible: %v", err)
	}
	return router.NewStatic(cfg, map[string]providerpkg.Provider{"openai_api": upstreamProvider})
}
