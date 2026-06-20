package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"battle-proxy-akira/internal/auth"
	"battle-proxy-akira/internal/config"
	"battle-proxy-akira/internal/ir"
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
		{name: "stream true", body: `{"model":"gpt-test","messages":[{"role":"user","content":"hello"}],"stream":true}`, wantStatus: http.StatusBadRequest, wantCode: ErrorInvalidRequest},
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
