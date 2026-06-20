package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"battle-proxy-akira/internal/config"
)

func TestChatCompletionsAcceptsBodyWithinConfiguredLimit(t *testing.T) {
	t.Parallel()

	logger := &recordingLogger{}
	handler := NewServer(
		WithServerConfig(config.ServerConfig{MaxBodyBytes: 1024}),
		WithChatRouter(&requestIDRouter{provider: &requestIDProvider{}}),
		WithRequestLogger(logger),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(logger.records) != 1 || logger.records[0].Status != http.StatusOK {
		t.Fatalf("log records = %#v", logger.records)
	}
}

func TestChatCompletionsRejectsBodyOverConfiguredLimit(t *testing.T) {
	t.Parallel()

	logger := &recordingLogger{}
	handler := NewServer(
		WithServerConfig(config.ServerConfig{MaxBodyBytes: 32}),
		WithChatRouter(&requestIDRouter{provider: &requestIDProvider{}}),
		WithRequestLogger(logger),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"this body is too large for the tiny configured limit"}]}`))
	handler.ServeHTTP(rec, req)

	assertTooLargeResponse(t, rec)
	if len(logger.records) != 1 || logger.records[0].Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("log records = %#v", logger.records)
	}
}

func TestChatCompletionsDefaultBodyLimitRejectsLargeContentLength(t *testing.T) {
	t.Parallel()

	handler := NewServer(WithChatRouter(&requestIDRouter{provider: &requestIDProvider{}}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	req.ContentLength = config.DefaultMaxBodyBytes + 1
	handler.ServeHTTP(rec, req)

	assertTooLargeResponse(t, rec)
}

func assertTooLargeResponse(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	var body OpenAIErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != string(ErrorInputTooLarge) {
		t.Fatalf("error code = %q, want %q", body.Error.Code, ErrorInputTooLarge)
	}
}
