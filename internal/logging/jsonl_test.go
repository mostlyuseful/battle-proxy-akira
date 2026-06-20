package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
