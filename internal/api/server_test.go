package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthAndReadinessEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		wantStatus string
	}{
		{name: "health", path: "/healthz", wantStatus: "ok"},
		{name: "readiness", path: "/readyz", wantStatus: "ready"},
	}

	handler := NewServer()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("content-type = %q, want application/json", got)
			}

			var body healthResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", body.Status, tt.wantStatus)
			}
		})
	}
}
