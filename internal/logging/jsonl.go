// Package logging provides request logging implementations.
package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	SessionID        string               `json:"session_id,omitempty"`
	Endpoint         string               `json:"endpoint,omitempty"`
	RequestedModel   string               `json:"requested_model"`
	ResolvedProvider string               `json:"resolved_provider"`
	ResolvedModel    string               `json:"resolved_model"`
	Stream           bool                 `json:"stream"`
	Status           int                  `json:"status"`
	LatencyMS        int64                `json:"latency_ms"`
	RetryCount       int                  `json:"retry_count"`
	ImageInputs      []ImageInputMetadata `json:"image_inputs,omitempty"`
	Transcript       any                  `json:"transcript,omitempty"`
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
	return NewWithLogger(cfg, nil)
}

// NewWithLogger returns a logger and optional verbose diagnostics.
func NewWithLogger(cfg config.LoggingConfig, logger *slog.Logger) (Logger, error) {
	mode := cfg.Mode
	if mode == "" {
		if cfg.Enabled {
			mode = config.LoggingModeMetadataOnly
		} else {
			mode = config.LoggingModeOff
		}
	}
	mode = canonicalMode(mode)
	if !cfg.Enabled || mode == config.LoggingModeOff {
		if logger != nil {
			logger.Info("request logging disabled", "configured_mode", mode)
		}
		return NoopLogger{}, nil
	}
	if mode != config.LoggingModeMetadataOnly && mode != config.LoggingModeInvasive {
		return nil, fmt.Errorf("unsupported logging mode %q", mode)
	}
	if cfg.Path == "" {
		return nil, fmt.Errorf("logging path is required for %s mode", mode)
	}
	if logger != nil {
		logger.Info("request logging configured", "path", cfg.Path, "mode", mode)
	}
	return &JSONLLogger{path: cfg.Path, mode: mode, logger: logger}, nil
}

// NoopLogger discards request records.
type NoopLogger struct{}

// Mode reports the effective logging mode.
func (NoopLogger) Mode() string { return config.LoggingModeOff }

// LogRequest implements Logger.
func (NoopLogger) LogRequest(context.Context, RequestLogRecord) error { return nil }

// JSONLLogger appends one JSON object per request to a local file.
type JSONLLogger struct {
	path   string
	mode   string
	mu     sync.Mutex
	logger *slog.Logger
}

// Mode reports the effective logging mode.
func (l *JSONLLogger) Mode() string {
	if l == nil {
		return config.LoggingModeOff
	}
	return canonicalMode(l.mode)
}

// LogRequest appends a log record to the configured JSONL file.
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
		if l.logger != nil {
			l.logger.Warn("open metadata log failed", "path", l.path, "error", err)
		}
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(encoded, '\n')); err != nil {
		if l.logger != nil {
			l.logger.Warn("write metadata log failed", "path", l.path, "error", err)
		}
		return err
	}
	if l.logger != nil {
		l.logger.Info("metadata request log written", "path", l.path, "request_id", rec.RequestID)
	}
	return nil
}
