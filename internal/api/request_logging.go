package api

import (
	"encoding/json"
	"net/http"

	requestlog "battle-proxy-akira/internal/logging"
)

const sessionIDHeader = "X-Session-Id"

func sessionIDForRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value := r.Header.Get(sessionIDHeader); isSafeRequestID(value) {
		return value
	}
	return ""
}

func newRequestLogRecord(r *http.Request, endpoint string, requestID string) requestlog.RequestLogRecord {
	return requestlog.RequestLogRecord{
		RequestID:  requestID,
		SessionID:  sessionIDForRequest(r),
		Endpoint:   endpoint,
		RetryCount: 0,
	}
}

func attachRequestTranscript(logger requestlog.Logger, rec *requestlog.RequestLogRecord, body []byte) {
	if rec == nil || !requestlog.CapturesTranscript(logger) {
		return
	}
	rec.Transcript = &requestlog.Transcript{Request: append(json.RawMessage(nil), body...)}
}

func appendTranscriptAttempt(rec *requestlog.RequestLogRecord, providerName, providerModel string) *requestlog.TranscriptAttempt {
	if rec == nil || rec.Transcript == nil {
		return nil
	}
	transcript, ok := rec.Transcript.(*requestlog.Transcript)
	if !ok || transcript == nil {
		return nil
	}
	transcript.Attempts = append(transcript.Attempts, requestlog.TranscriptAttempt{Provider: providerName, Model: providerModel})
	return &transcript.Attempts[len(transcript.Attempts)-1]
}
