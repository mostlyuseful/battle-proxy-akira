package logging

import (
	"encoding/json"
	"regexp"
)

const redacted = "[REDACTED]"

var (
	bearerSecretPattern = regexp.MustCompile(`(?i)bearer\s+[^\s,;"')\]}]+`)
	apiKeyPattern       = regexp.MustCompile(`sk-[^\s,;"')\]}]+`)
)

// RedactString removes common bearer/API-key secret patterns from a string.
func RedactString(s string) string {
	s = bearerSecretPattern.ReplaceAllString(s, "Bearer "+redacted)
	s = apiKeyPattern.ReplaceAllString(s, redacted)
	return s
}

// RedactRecord applies baseline redaction to log records.
func RedactRecord(rec RequestLogRecord) RequestLogRecord {
	rec.RequestID = RedactString(rec.RequestID)
	rec.SessionID = RedactString(rec.SessionID)
	rec.Endpoint = RedactString(rec.Endpoint)
	rec.RequestedModel = RedactString(rec.RequestedModel)
	rec.ResolvedProvider = RedactString(rec.ResolvedProvider)
	rec.ResolvedModel = RedactString(rec.ResolvedModel)
	rec.Transcript = redactValue(rec.Transcript)
	return rec
}

func redactValue(v any) any {
	switch value := v.(type) {
	case nil:
		return nil
	case string:
		return RedactString(value)
	case json.RawMessage:
		return redactJSON(value)
	case []byte:
		return redactJSON(value)
	case []any:
		out := make([]any, len(value))
		for i := range value {
			out[i] = redactValue(value[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = redactValue(item)
		}
		return out
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return RedactString("[UNSERIALIZABLE]")
		}
		return redactJSON(encoded)
	}
}

func redactJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return RedactString(string(raw))
	}
	return redactValue(decoded)
}
