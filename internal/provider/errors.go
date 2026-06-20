package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	ErrorInvalidRequest      = "invalid_request"
	ErrorUnsupportedModality = "unsupported_modality"
	ErrorInputTooLarge       = "input_too_large"
	ErrorProviderAuthFailed  = "provider_auth_failed"
	ErrorProviderRateLimited = "provider_rate_limited"
	ErrorProviderExhausted   = "provider_exhausted"
	ErrorPolicyDenied        = "policy_denied"
	ErrorUpstream            = "upstream_error"
)

// Error is a provider-neutral classified failure for router retry/fallback decisions.
type Error struct {
	Code       string
	Retryable  bool
	StatusCode int
	Provider   string
	RetryAfter *time.Time
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("provider %q error %s (status %d)", e.Provider, e.Code, e.StatusCode)
	}
	return fmt.Sprintf("provider %q error %s", e.Provider, e.Code)
}

// IsRetryable reports whether err is a classified provider failure safe for retry/fallback.
func IsRetryable(err error) bool {
	var providerErr *Error
	return errors.As(err, &providerErr) && providerErr.Retryable
}

// ErrorCode returns a stable provider-neutral error code for err when available.
func ErrorCode(err error) string {
	var providerErr *Error
	if errors.As(err, &providerErr) {
		return providerErr.Code
	}
	return ""
}

func classifyHTTPStatus(providerName string, status int, header http.Header, body []byte) *Error {
	code := codeFromStatus(status)
	if payloadCode := codeFromErrorPayload(body); payloadCode != "" {
		code = payloadCode
	}
	return &Error{
		Code:       code,
		Retryable:  retryableStatus(status),
		StatusCode: status,
		Provider:   providerName,
		RetryAfter: parseRetryAfter(header.Get("Retry-After")),
	}
}

func classifyNetworkError(providerName string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{
		Code:      ErrorUpstream,
		Retryable: isRetryableNetworkError(err),
		Provider:  providerName,
	}
}

func codeFromStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return ErrorInvalidRequest
	case http.StatusUnauthorized:
		return ErrorProviderAuthFailed
	case http.StatusForbidden:
		return ErrorPolicyDenied
	case http.StatusRequestEntityTooLarge:
		return ErrorInputTooLarge
	case http.StatusUnprocessableEntity:
		return ErrorUnsupportedModality
	case http.StatusTooManyRequests:
		return ErrorProviderRateLimited
	default:
		return ErrorUpstream
	}
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func codeFromErrorPayload(body []byte) string {
	if len(body) == 0 || !json.Valid(body) {
		return ""
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	payload := strings.ToLower(envelope.Error.Code)
	if payload == "" {
		payload = strings.ToLower(envelope.Error.Type)
	}
	switch payload {
	case "invalid_request", "invalid_request_error", "bad_request":
		return ErrorInvalidRequest
	case "unsupported_modality", "unsupported_media_type":
		return ErrorUnsupportedModality
	case "context_length_exceeded", "content_length_exceeded", "input_too_large", "request_too_large":
		return ErrorInputTooLarge
	case "rate_limit_exceeded", "rate_limited", "too_many_requests":
		return ErrorProviderRateLimited
	case "provider_exhausted", "insufficient_quota", "quota_exceeded", "billing_hard_limit_reached":
		return ErrorProviderExhausted
	case "authentication_error", "invalid_api_key", "provider_auth_failed":
		return ErrorProviderAuthFailed
	case "permission_error", "policy_denied", "forbidden":
		return ErrorPolicyDenied
	default:
		return ""
	}
}

func parseRetryAfter(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds < 0 {
			seconds = 0
		}
		at := time.Now().Add(time.Duration(seconds) * time.Second)
		return &at
	}
	if at, err := http.ParseTime(raw); err == nil {
		return &at
	}
	return nil
}

func isRetryableNetworkError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") || strings.Contains(msg, "connection refused")
}
