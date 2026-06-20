package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusForErrorCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code ErrorCode
		want int
	}{
		{code: ErrorInvalidRequest, want: http.StatusBadRequest},
		{code: ErrorUnknownModel, want: http.StatusNotFound},
		{code: ErrorNoAvailableModel, want: http.StatusServiceUnavailable},
		{code: ErrorUnsupportedModality, want: http.StatusUnprocessableEntity},
		{code: ErrorUpstream, want: http.StatusBadGateway},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			t.Parallel()

			if got := StatusForErrorCode(tt.code); got != tt.want {
				t.Fatalf("StatusForErrorCode(%q) = %d, want %d", tt.code, got, tt.want)
			}
		})
	}
}

func TestWriteOpenAIError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	WriteOpenAIError(rec, NewProxyError(ErrorNoAvailableModel, "No available provider for synthetic model coding", "model"))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}

	var body OpenAIErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Error.Message != "No available provider for synthetic model coding" {
		t.Fatalf("message = %q", body.Error.Message)
	}
	if body.Error.Type != "proxy_routing_error" {
		t.Fatalf("type = %q, want proxy_routing_error", body.Error.Type)
	}
	if body.Error.Param == nil || *body.Error.Param != "model" {
		t.Fatalf("param = %v, want model", body.Error.Param)
	}
	if body.Error.Code != string(ErrorNoAvailableModel) {
		t.Fatalf("code = %q, want %q", body.Error.Code, ErrorNoAvailableModel)
	}
}

func TestWriteOpenAIErrorWithoutParamUsesNullParam(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	WriteOpenAIError(rec, NewProxyError(ErrorUpstream, "upstream failed", ""))

	var raw map[string]map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := raw["error"]["param"]; !ok {
		t.Fatal("error.param missing, want explicit null field")
	}
	if raw["error"]["param"] != nil {
		t.Fatalf("error.param = %#v, want nil", raw["error"]["param"])
	}
}
