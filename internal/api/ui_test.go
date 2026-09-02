package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"battle-proxy-akira/internal/config"
)

func TestUIPageServesHTML(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/", "/ui"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		NewServer().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("path %s status code = %d, want %d", path, rec.Code, http.StatusOK)
		}
		if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
			t.Fatalf("path %s content-type = %q", path, got)
		}
		if !strings.Contains(rec.Body.String(), "llm-proxy UI") {
			t.Fatalf("path %s body = %q", path, rec.Body.String())
		}
		for _, want := range []string{"log-card", "renderSummary", "renderChatLite", "raw-json", "tab-logs", "tab-models", "activateTab('logs')", "modelsLoaded = false", "!showLogs && !modelsLoaded", "remember-token", "restoreRememberedToken", "persistRememberedToken", "localStorage", "body.reset", "logsLoading", "pendingLogsReset"} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("path %s body missing %q in %q", path, want, rec.Body.String())
			}
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
	cursor, valid := parseLogCursor(body.Cursor)
	if !body.Enabled || !valid || cursor.offset != int64(len("one\ntwo\n")) || len(body.Lines) != 2 || body.Lines[0] != "one" || body.Lines[1] != "two" {
		t.Fatalf("body = %#v, parsed cursor = %#v, valid = %t", body, cursor, valid)
	}
}

func TestUILogsEndpointReturnsEmptyLogBeforeFirstRequest(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "not-created-yet.jsonl")
	handler := NewServer(
		WithLoggingConfig(config.LoggingConfig{Enabled: true, Path: path}),
		WithClientAuth(StaticBearerAuth([]string{"token"})),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/api/logs", nil)
	req.Header.Set("Authorization", "Bearer token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body logsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}
	cursor, valid := parseLogCursor(body.Cursor)
	if !body.Enabled || !valid || cursor.offset != 0 || body.Reset || len(body.Lines) != 0 {
		t.Fatalf("body = %#v, parsed cursor = %#v, valid = %t; want enabled empty log", body, cursor, valid)
	}
}

func TestUILogsEndpointHandlesLargeLogEntries(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	largeLine := strings.Repeat("x", 512*1024)
	if err := os.WriteFile(path, []byte(largeLine+"\ntwo\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	handler := NewServer(
		WithLoggingConfig(config.LoggingConfig{Enabled: true, Path: path}),
		WithClientAuth(StaticBearerAuth([]string{"token"})),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/api/logs", nil)
	req.Header.Set("Authorization", "Bearer token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body logsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}
	cursor, valid := parseLogCursor(body.Cursor)
	if !body.Enabled || !valid || cursor.offset != int64(len(largeLine)+len("\ntwo\n")) || len(body.Lines) != 2 || body.Lines[0] != largeLine || body.Lines[1] != "two" {
		t.Fatalf("body = %#v, parsed cursor = %#v, valid = %t", body, cursor, valid)
	}
}

func TestUILogsEndpointReturnsOnlyAppendedLinesAfterCursor(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	handler := NewServer(
		WithLoggingConfig(config.LoggingConfig{Enabled: true, Path: path}),
		WithClientAuth(StaticBearerAuth([]string{"token"})),
	)

	first := requestLogs(t, handler, "0")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open log for append: %v", err)
	}
	if _, err := f.WriteString("three\n"); err != nil {
		f.Close()
		t.Fatalf("append log: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close log: %v", err)
	}
	body := requestLogs(t, handler, first.Cursor)
	cursor, valid := parseLogCursor(body.Cursor)
	if !body.Enabled || !valid || body.Reset || cursor.offset != int64(len("one\ntwo\nthree\n")) || len(body.Lines) != 1 || body.Lines[0] != "three" {
		t.Fatalf("body = %#v, parsed cursor = %#v, valid = %t", body, cursor, valid)
	}
}

func TestUILogsEndpointRejectsUnsafeLegacyLineCursor(t *testing.T) {
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
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "click Load logs") {
		t.Fatalf("status = %d, body = %s; want cursor conflict", rec.Code, rec.Body.String())
	}
}

func TestUILogsEndpointDoesNotSkipBacklogBeyondPageLimit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	content := strings.Repeat("line\n", 205)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	handler := NewServer(
		WithLoggingConfig(config.LoggingConfig{Enabled: true, Path: path}),
		WithClientAuth(StaticBearerAuth([]string{"token"})),
	)

	first := requestLogs(t, handler, "0")
	if len(first.Lines) != 200 {
		t.Fatalf("first page line count = %d, want 200", len(first.Lines))
	}
	second := requestLogs(t, handler, first.Cursor)
	cursor, valid := parseLogCursor(second.Cursor)
	if !valid || second.Reset || len(second.Lines) != 5 || cursor.offset != int64(len(content)) {
		t.Fatalf("second page = %#v, parsed cursor = %#v, valid = %t", second, cursor, valid)
	}
}

func TestUILogsEndpointResetsWhenLogIsReplaced(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "requests.jsonl")
	sameTail := strings.Repeat("x", 80)
	if err := os.WriteFile(path, []byte("old\n"+sameTail+"\n"), 0o600); err != nil {
		t.Fatalf("write old log: %v", err)
	}
	handler := NewServer(
		WithLoggingConfig(config.LoggingConfig{Enabled: true, Path: path}),
		WithClientAuth(StaticBearerAuth([]string{"token"})),
	)
	first := requestLogs(t, handler, "0")

	replacementPath := filepath.Join(dir, "replacement.jsonl")
	if err := os.WriteFile(replacementPath, []byte("new\n"+sameTail+"\n"), 0o600); err != nil {
		t.Fatalf("write replacement log: %v", err)
	}
	if err := os.Rename(replacementPath, path); err != nil {
		t.Fatalf("replace log: %v", err)
	}
	body := requestLogs(t, handler, first.Cursor)
	if !body.Reset || len(body.Lines) != 2 || body.Lines[0] != "new" || body.Lines[1] != sameTail {
		t.Fatalf("body = %#v, want reset replacement log", body)
	}
}

func TestUILogsEndpointResetsWhenLogIsRewrittenInPlace(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	unchangedTail := strings.Repeat("x", 100) + "\n"
	oldContent := "old\n" + unchangedTail
	if err := os.WriteFile(path, []byte(oldContent), 0o600); err != nil {
		t.Fatalf("write old log: %v", err)
	}
	handler := NewServer(
		WithLoggingConfig(config.LoggingConfig{Enabled: true, Path: path}),
		WithClientAuth(StaticBearerAuth([]string{"token"})),
	)
	first := requestLogs(t, handler, "0")

	newContent := "new\n" + unchangedTail + "extra\n"
	if err := os.WriteFile(path, []byte(newContent), 0o600); err != nil {
		t.Fatalf("rewrite log: %v", err)
	}
	body := requestLogs(t, handler, first.Cursor)
	if !body.Reset || len(body.Lines) != 3 || body.Lines[0] != "new" || body.Lines[2] != "extra" {
		t.Fatalf("body = %#v, want reset rewritten log", body)
	}
}

func TestUILogsEndpointResetsWhenLogIsDeleted(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("write old log: %v", err)
	}
	handler := NewServer(
		WithLoggingConfig(config.LoggingConfig{Enabled: true, Path: path}),
		WithClientAuth(StaticBearerAuth([]string{"token"})),
	)
	first := requestLogs(t, handler, "0")

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove log: %v", err)
	}
	missing := requestLogs(t, handler, first.Cursor)
	missingCursor, valid := parseLogCursor(missing.Cursor)
	if !missing.Reset || !valid || missingCursor.offset != 0 || len(missing.Lines) != 0 {
		t.Fatalf("missing body = %#v, parsed cursor = %#v, valid = %t; want reset empty log", missing, missingCursor, valid)
	}

	if err := os.WriteFile(path, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("recreate log: %v", err)
	}
	recreated := requestLogs(t, handler, missing.Cursor)
	if recreated.Reset || len(recreated.Lines) != 1 || recreated.Lines[0] != "new" {
		t.Fatalf("recreated body = %#v", recreated)
	}
}

func TestUILogsEndpointWaitsForCompleteAppendedLine(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	if err := os.WriteFile(path, []byte("one\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	handler := NewServer(
		WithLoggingConfig(config.LoggingConfig{Enabled: true, Path: path}),
		WithClientAuth(StaticBearerAuth([]string{"token"})),
	)
	first := requestLogs(t, handler, "0")

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open log for append: %v", err)
	}
	if _, err := f.WriteString("partial"); err != nil {
		f.Close()
		t.Fatalf("append partial line: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close log: %v", err)
	}
	partial := requestLogs(t, handler, first.Cursor)
	if partial.Cursor != first.Cursor || len(partial.Lines) != 0 {
		t.Fatalf("partial body = %#v, want unchanged cursor", partial)
	}

	f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("reopen log for append: %v", err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		f.Close()
		t.Fatalf("complete line: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close completed log: %v", err)
	}
	complete := requestLogs(t, handler, partial.Cursor)
	if len(complete.Lines) != 1 || complete.Lines[0] != "partial" {
		t.Fatalf("complete body = %#v", complete)
	}
}

func requestLogs(t *testing.T, handler http.Handler, cursor string) logsResponse {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/api/logs?after="+url.QueryEscape(cursor), nil)
	req.Header.Set("Authorization", "Bearer token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body logsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}
	return body
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
