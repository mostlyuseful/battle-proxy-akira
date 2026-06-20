package logging

import "regexp"

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

// RedactRecord applies baseline redaction to metadata-only log records.
func RedactRecord(rec RequestLogRecord) RequestLogRecord {
	rec.RequestID = RedactString(rec.RequestID)
	rec.RequestedModel = RedactString(rec.RequestedModel)
	rec.ResolvedProvider = RedactString(rec.ResolvedProvider)
	rec.ResolvedModel = RedactString(rec.ResolvedModel)
	return rec
}
