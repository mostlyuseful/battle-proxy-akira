package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"battle-proxy-akira/internal/config"
)

func TestStaticBearerAuthAcceptsConfiguredToken(t *testing.T) {
	t.Parallel()

	handler := StaticBearerAuth([]string{"local-dev-token"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer local-dev-token")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestStaticBearerAuthRejectsMissingOrInvalidToken(t *testing.T) {
	t.Parallel()

	handler := StaticBearerAuth([]string{"local-dev-token"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	for _, tt := range []struct {
		name   string
		header string
	}{
		{name: "missing"},
		{name: "wrong", header: "Bearer wrong-token"},
		{name: "wrong scheme", header: "Basic local-dev-token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status code = %d, want %d", rec.Code, http.StatusForbidden)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got == "" {
				t.Fatal("WWW-Authenticate header missing")
			}
			var body OpenAIErrorResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if body.Error.Code != string(ErrorPolicyDenied) {
				t.Fatalf("error code = %q, want %q", body.Error.Code, ErrorPolicyDenied)
			}
			if strings.Contains(body.Error.Message, "local-dev-token") || strings.Contains(body.Error.Message, "wrong-token") {
				t.Fatalf("error leaked token value: %q", body.Error.Message)
			}
		})
	}
}

func TestNewClientAuthMiddlewareModeNoneDisablesAuth(t *testing.T) {
	t.Parallel()

	middleware, err := NewClientAuthMiddleware(config.ClientAuthConfig{Mode: config.ClientAuthModeNone})
	if err != nil {
		t.Fatalf("NewClientAuthMiddleware: %v", err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusAccepted)
	}
}

func TestNewClientAuthMiddlewareReadsStaticBearerTokensFromEnv(t *testing.T) {
	t.Setenv("LLM_PROXY_CLIENT_TOKENS", " first-token, second-token ")

	middleware, err := NewClientAuthMiddleware(config.ClientAuthConfig{
		Mode:      config.ClientAuthModeStaticBearer,
		TokensEnv: "LLM_PROXY_CLIENT_TOKENS",
	})
	if err != nil {
		t.Fatalf("NewClientAuthMiddleware: %v", err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer second-token")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestNewClientAuthMiddlewareReadsStaticBearerTokensFromConfigValue(t *testing.T) {
	t.Parallel()

	middleware, err := NewClientAuthMiddleware(config.ClientAuthConfig{
		Mode:      config.ClientAuthModeStaticBearer,
		TokensVal: "first-token",
	})
	if err != nil {
		t.Fatalf("NewClientAuthMiddleware: %v", err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer first-token")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestNewClientAuthMiddlewareRejectsMissingStaticBearerConfig(t *testing.T) {
	t.Parallel()

	_, err := NewClientAuthMiddleware(config.ClientAuthConfig{
		Mode: config.ClientAuthModeStaticBearer,
	})
	if err == nil {
		t.Fatal("NewClientAuthMiddleware returned nil error, want config error")
	}
}

func TestNewClientAuthMiddlewareRejectsEmptyTokenEnvWithoutLeakingValues(t *testing.T) {
	t.Setenv("LLM_PROXY_CLIENT_TOKENS", " , ")

	_, err := NewClientAuthMiddleware(config.ClientAuthConfig{
		Mode:      config.ClientAuthModeStaticBearer,
		TokensEnv: "LLM_PROXY_CLIENT_TOKENS",
	})
	if err == nil {
		t.Fatal("NewClientAuthMiddleware returned nil error, want config error")
	}
	if strings.Contains(err.Error(), "LLM_PROXY_CLIENT_TOKENS") {
		t.Fatalf("error should not include env var name or token values: %q", err.Error())
	}
}

func TestClientAuthMiddlewareProtectsModelsEndpoint(t *testing.T) {
	t.Setenv("LLM_PROXY_CLIENT_TOKENS", "local-dev-token")
	middleware, err := NewClientAuthMiddleware(config.ClientAuthConfig{
		Mode:      config.ClientAuthModeBearerTokens,
		TokensEnv: "LLM_PROXY_CLIENT_TOKENS",
	})
	if err != nil {
		t.Fatalf("NewClientAuthMiddleware: %v", err)
	}
	handler := NewServer(WithClientAuth(middleware))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated status code = %d, want %d", rec.Code, http.StatusForbidden)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer local-dev-token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated status code = %d, want %d", rec.Code, http.StatusOK)
	}
}
