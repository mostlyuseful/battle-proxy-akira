// Package logging provides request logging implementations.
package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"battle-proxy-akira/internal/config"
)

// Logger records request metadata.
type Logger interface {
	LogRequest(ctx context.Context, rec RequestLogRecord) error
}

// RequestLogRecord is the metadata-only JSONL shape for one completed request.
type RequestLogRecord struct {
	Timestamp        time.Time            `json:"ts"`
	RequestID        string               `json:"request_id"`
	RequestedModel   string               `json:"requested_model"`
	ResolvedProvider string               `json:"resolved_provider"`
	ResolvedModel    string               `json:"resolved_model"`
	Stream           bool                 `json:"stream"`
	Status           int                  `json:"status"`
	LatencyMS        int64                `json:"latency_ms"`
	RetryCount       int                  `json:"retry_count"`
	ImageInputs      []ImageInputMetadata `json:"image_inputs,omitempty"`
	Transcript       any                  `json:"transcript"`
}

// ImageInputMetadata is safe-to-log metadata for one image input.
type ImageInputMetadata struct {
	Source      string `json:"source"`
	MIMEType    string `json:"mime_type,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	ByteLength  int    `json:"byte_length,omitempty"`
	URLRedacted bool   `json:"url_redacted,omitempty"`
}

// New returns a logger for the configured mode. off returns a no-op logger.
func New(cfg config.LoggingConfig) (Logger, error) {
	mode := cfg.Mode
	if mode == "" {
		if cfg.Enabled {
			mode = config.LoggingModeMetadataOnly
		} else {
			mode = config.LoggingModeOff
		}
	}
	if !cfg.Enabled || mode == config.LoggingModeOff {
		return NoopLogger{}, nil
	}
	if mode != config.LoggingModeMetadataOnly {
		return nil, fmt.Errorf("unsupported logging mode %q", mode)
	}
	if cfg.Path == "" {
		return nil, fmt.Errorf("logging path is required for metadata_only mode")
	}
	return &JSONLLogger{path: cfg.Path}, nil
}

// NoopLogger discards request records.
type NoopLogger struct{}

// LogRequest implements Logger.
func (NoopLogger) LogRequest(context.Context, RequestLogRecord) error { return nil }

// JSONLLogger appends one JSON object per request to a local file.
type JSONLLogger struct {
	path string
	mu   sync.Mutex
}

// LogRequest appends a metadata log record to the configured JSONL file.
func (l *JSONLLogger) LogRequest(ctx context.Context, rec RequestLogRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l == nil || l.path == "" {
		return nil
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}
	rec = RedactRecord(rec)

	encoded, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}
