package logging

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"battle-proxy-akira/internal/config"
	"battle-proxy-akira/internal/ir"
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

func TestImageMetadataFromRequestHashesDataURLsAndRedactsExternalURLs(t *testing.T) {
	t.Parallel()

	data := []byte("image-bytes")
	hash := sha256.Sum256(data)
	req := ir.Request{Messages: []ir.Message{{Content: []ir.ContentPart{
		{Type: ir.ContentTypeImageURL, ImageURL: "data:image/png;base64,aW1hZ2UtYnl0ZXM="},
		{Type: ir.ContentTypeImageURL, ImageURL: "https://example.test/image.png"},
	}}}}

	got := ImageMetadataFromRequest(req)
	if len(got) != 2 {
		t.Fatalf("metadata length = %d, want 2: %#v", len(got), got)
	}
	if got[0].Source != ImageSourceDataURL || got[0].MIMEType != "image/png" || got[0].ByteLength != len(data) || got[0].SHA256 != hex.EncodeToString(hash[:]) {
		t.Fatalf("data URL metadata = %#v", got[0])
	}
	if got[1].Source != ImageSourceURL || !got[1].URLRedacted || got[1].SHA256 != "" {
		t.Fatalf("external URL metadata = %#v", got[1])
	}
}

func TestJSONLLoggerWritesImageMetadataWithoutRawDataURL(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	logger, err := New(config.LoggingConfig{Enabled: true, Mode: config.LoggingModeMetadataOnly, Path: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rawBase64 := "aW1hZ2UtYnl0ZXM="
	rec := RequestLogRecord{
		Timestamp:   time.Unix(123, 0).UTC(),
		RequestID:   "req_image",
		Status:      200,
		ImageInputs: ImageMetadataFromRequest(ir.Request{Messages: []ir.Message{{Content: []ir.ContentPart{{Type: ir.ContentTypeImageURL, ImageURL: "data:image/png;base64," + rawBase64}}}}}),
	}
	if err := logger.LogRequest(context.Background(), rec); err != nil {
		t.Fatalf("LogRequest: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if strings.Contains(string(data), rawBase64) || strings.Contains(string(data), "data:image/png") {
		t.Fatalf("log output leaked raw data URL: %s", data)
	}
	var got RequestLogRecord
	if err := json.Unmarshal(bytes.TrimSpace(data), &got); err != nil {
		t.Fatalf("unmarshal log: %v", err)
	}
	if len(got.ImageInputs) != 1 || got.ImageInputs[0].SHA256 == "" || got.ImageInputs[0].ByteLength != len("image-bytes") || got.ImageInputs[0].MIMEType != "image/png" {
		t.Fatalf("image metadata = %#v", got.ImageInputs)
	}
}

func TestJSONLLoggerWritesInvasiveTranscriptAndRedactsSecrets(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "requests.jsonl")
	logger, err := New(config.LoggingConfig{Enabled: true, Mode: config.LoggingModeInvasive, Path: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	transcript := &Transcript{
		Request:  json.RawMessage(`{"authorization":"Bearer client-secret-token","prompt":"use sk-upstream-secret-token"}`),
		Attempts: []TranscriptAttempt{{Provider: "openai_api", Model: "gpt-test", Response: json.RawMessage(`{"output":"Bearer another-secret"}`)}},
	}
	if err := logger.LogRequest(context.Background(), RequestLogRecord{
		Timestamp:  time.Unix(123, 0).UTC(),
		RequestID:  "req_test",
		SessionID:  "sess_123",
		Endpoint:   "chat_completions",
		Status:     200,
		Transcript: transcript,
	}); err != nil {
		t.Fatalf("LogRequest: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	for _, secret := range []string{"client-secret-token", "sk-upstream-secret-token", "another-secret"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("log output leaked %q in %s", secret, data)
		}
	}
	if !strings.Contains(string(data), "session_id") || !strings.Contains(string(data), "transcript") {
		t.Fatalf("log output missing session or transcript: %s", data)
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
