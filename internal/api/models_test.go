package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"battle-proxy-akira/internal/ir"
)

func TestModelsEndpointReturnsOpenAICompatibleList(t *testing.T) {
	t.Parallel()

	handler := NewServer(WithModelLister(ModelListerFunc(func(context.Context) ([]ir.Model, error) {
		return []ir.Model{
			{ID: "gpt-test", Provider: "openai_api", Name: "gpt-test"},
			{ID: "coding", Provider: "proxy", Name: "coding", Synthetic: true},
		}, nil
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}

	var body modelListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Object != "list" {
		t.Fatalf("object = %q, want list", body.Object)
	}
	if len(body.Data) != 2 {
		t.Fatalf("data length = %d, want 2", len(body.Data))
	}
	if body.Data[0] != (modelResponse{ID: "coding", Object: "model", Created: 0, OwnedBy: "proxy"}) {
		t.Fatalf("data[0] = %#v", body.Data[0])
	}
	if body.Data[1] != (modelResponse{ID: "gpt-test", Object: "model", Created: 0, OwnedBy: "openai_api"}) {
		t.Fatalf("data[1] = %#v", body.Data[1])
	}
}

func TestModelsEndpointAppliesClientAuthMiddleware(t *testing.T) {
	t.Parallel()

	called := false
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			if r.Header.Get("Authorization") != "Bearer ok" {
				WriteOpenAIError(w, NewProxyError(ErrorInvalidRequest, "missing bearer", ""))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	handler := NewServer(
		WithModelLister(ModelListerFunc(emptyModels)),
		WithClientAuth(auth),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("auth middleware was not called")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer ok")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authorized status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestModelsEndpointReturnsOpenAIErrorOnListerFailure(t *testing.T) {
	t.Parallel()

	handler := NewServer(WithModelLister(ModelListerFunc(func(context.Context) ([]ir.Model, error) {
		return nil, errors.New("boom")
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	var body OpenAIErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != string(ErrorUpstream) {
		t.Fatalf("error code = %q, want %q", body.Error.Code, ErrorUpstream)
	}
}

func TestNewServerRegistersModelsEndpointWithEmptyDefault(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	NewServer().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var body modelListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Object != "list" || len(body.Data) != 0 {
		t.Fatalf("body = %#v, want empty model list", body)
	}
}
