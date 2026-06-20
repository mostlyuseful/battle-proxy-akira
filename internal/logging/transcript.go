package logging

import (
	"encoding/json"

	"battle-proxy-akira/internal/config"
)

// ModeReporter exposes the effective logging mode for request handlers.
type ModeReporter interface {
	Mode() string
}

// ModeOf reports the effective logger mode, or off if unknown.
func ModeOf(logger Logger) string {
	reporter, ok := logger.(ModeReporter)
	if !ok || reporter == nil {
		return config.LoggingModeOff
	}
	return canonicalMode(reporter.Mode())
}

// CapturesTranscript reports whether the logging mode should include transcripts.
func CapturesTranscript(logger Logger) bool {
	switch ModeOf(logger) {
	case config.LoggingModeInvasive:
		return true
	default:
		return false
	}
}

func canonicalMode(mode string) string {
	switch mode {
	case config.LoggingModeFullTranscript, config.LoggingModeFullTranscriptPerRequest:
		return config.LoggingModeInvasive
	case "":
		return config.LoggingModeOff
	default:
		return mode
	}
}

// Transcript captures request/response details for one proxied request.
type Transcript struct {
	Request  json.RawMessage     `json:"request,omitempty"`
	Attempts []TranscriptAttempt `json:"attempts,omitempty"`
}

// TranscriptAttempt captures one provider attempt, including retries.
type TranscriptAttempt struct {
	Provider string            `json:"provider,omitempty"`
	Model    string            `json:"model,omitempty"`
	Response json.RawMessage   `json:"response,omitempty"`
	Stream   []json.RawMessage `json:"stream,omitempty"`
	Error    string            `json:"error,omitempty"`
}
