package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

const requestIDHeader = "X-Request-ID"

type requestIDContextKey struct{}

// ContextWithRequestID returns a context carrying the per-request correlation ID.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

// RequestIDFromContext returns the per-request correlation ID, if one is present.
func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := requestIDFromHeaderOrNew(r.Header.Get(requestIDHeader))
		w.Header().Set(requestIDHeader, requestID)
		next.ServeHTTP(w, r.WithContext(ContextWithRequestID(r.Context(), requestID)))
	})
}

func requestIDForRequest(r *http.Request) string {
	if requestID := RequestIDFromContext(r.Context()); requestID != "" {
		return requestID
	}
	return requestIDFromHeaderOrNew(r.Header.Get(requestIDHeader))
}

func requestIDFromHeaderOrNew(headerValue string) string {
	if isSafeRequestID(headerValue) {
		return headerValue
	}
	return newRequestID()
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return "req_" + hex.EncodeToString(b[:])
}

func isSafeRequestID(requestID string) bool {
	if requestID == "" || len(requestID) > 128 {
		return false
	}
	lower := strings.ToLower(requestID)
	if strings.Contains(lower, "bearer") || strings.Contains(lower, "sk-") {
		return false
	}
	for _, r := range requestID {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == ':':
		default:
			return false
		}
	}
	return true
}
