package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"battle-proxy-akira/internal/config"
)

func TestUIPageServesHTML(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	NewServer().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("content-type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "llm-proxy UI") {
		t.Fatalf("body = %q", rec.Body.String())
	}
	for _, want := range []string{"log-card", "renderSummary", "Transcript"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body missing %q in %q", want, rec.Body.String())
		}
	}
}

func TestUILogsEndpointRequiresClientAuth(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	handler := NewServer(
		WithLoggingConfig(config.LoggingConfig{Enabled: true, Path: path}),
		WithClientAuth(StaticBearerAuth([]string{"token"})),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/api/logs", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated status code = %d, want %d", rec.Code, http.StatusForbidden)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/api/logs", nil)
	req.Header.Set("Authorization", "Bearer token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var body logsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}
	if !body.Enabled || body.Cursor != 2 || len(body.Lines) != 2 || body.Lines[0] != "one" || body.Lines[1] != "two" {
		t.Fatalf("body = %#v", body)
	}
}

func TestUILogsEndpointReturnsOnlyAppendedLinesAfterCursor(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	handler := NewServer(
		WithLoggingConfig(config.LoggingConfig{Enabled: true, Path: path}),
		WithClientAuth(StaticBearerAuth([]string{"token"})),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/api/logs?after=2", nil)
	req.Header.Set("Authorization", "Bearer token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var body logsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}
	if !body.Enabled || body.Cursor != 3 || len(body.Lines) != 1 || body.Lines[0] != "three" {
		t.Fatalf("body = %#v", body)
	}
}

func TestUILogsEndpointReportsDisabledWhenLoggingOff(t *testing.T) {
	t.Parallel()

	handler := NewServer(WithClientAuth(StaticBearerAuth([]string{"token"})))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/api/logs", nil)
	req.Header.Set("Authorization", "Bearer token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var body logsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}
	if body.Enabled {
		t.Fatalf("body = %#v, want disabled", body)
	}
}
