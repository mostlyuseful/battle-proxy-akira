package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"battle-proxy-akira/internal/config"
)

func TestNewOffLoggerDiscardsRecords(t *testing.T) {
	t.Parallel()

	logger, err := New(config.LoggingConfig{Enabled: false, Mode: config.LoggingModeOff})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := logger.LogRequest(context.Background(), RequestLogRecord{RequestedModel: "coding"}); err != nil {
		t.Fatalf("LogRequest: %v", err)
	}
}

func TestRedactStringRemovesBearerAndAPIKeyPatterns(t *testing.T) {
	t.Parallel()

	input := "Authorization: Bearer client-secret-token uses sk-upstream-secret-token"
	got := RedactString(input)
	for _, secret := range []string{"client-secret-token", "sk-upstream-secret-token"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted string %q still contains secret %q", got, secret)
		}
	}
}

func TestJSONLLoggerWritesMetadataRecord(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	logger, err := New(config.LoggingConfig{Enabled: true, Mode: config.LoggingModeMetadataOnly, Path: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rec := RequestLogRecord{
		Timestamp:        time.Unix(123, 0).UTC(),
		RequestID:        "req_test",
		RequestedModel:   "coding",
		ResolvedProvider: "openai_api",
		ResolvedModel:    "gpt-test",
		Stream:           true,
		Status:           200,
		LatencyMS:        42,
		RetryCount:       1,
	}
	if err := logger.LogRequest(context.Background(), rec); err != nil {
		t.Fatalf("LogRequest: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatalf("missing first log line: %v", scanner.Err())
	}
	line := scanner.Text()
	if scanner.Scan() {
		t.Fatalf("unexpected second log line: %q", scanner.Text())
	}

	var got RequestLogRecord
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal log line: %v", err)
	}
	if got.RequestID != rec.RequestID || got.RequestedModel != rec.RequestedModel || got.ResolvedProvider != rec.ResolvedProvider || got.ResolvedModel != rec.ResolvedModel {
		t.Fatalf("log record = %#v", got)
	}
	if !got.Stream || got.Status != 200 || got.LatencyMS != 42 || got.RetryCount != 1 {
		t.Fatalf("log metrics = %#v", got)
	}
	if got.Transcript != nil {
		t.Fatalf("transcript = %#v, want nil", got.Transcript)
	}
}

func TestJSONLLoggerRedactsSecretLikeMetadata(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	logger, err := New(config.LoggingConfig{Enabled: true, Mode: config.LoggingModeMetadataOnly, Path: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := logger.LogRequest(context.Background(), RequestLogRecord{
		Timestamp:      time.Unix(123, 0).UTC(),
		RequestID:      "Bearer client-secret-token",
		RequestedModel: "sk-upstream-secret-token",
		Status:         502,
	}); err != nil {
		t.Fatalf("LogRequest: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	for _, secret := range []string{"client-secret-token", "sk-upstream-secret-token"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("log output leaked %q in %s", secret, data)
		}
	}
}
