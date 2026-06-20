package api

import "net/http"

// ErrorCode identifies a proxy error in OpenAI-compatible responses.
type ErrorCode string

const (
	ErrorInvalidRequest      ErrorCode = "invalid_request"
	ErrorUnsupportedModality ErrorCode = "unsupported_modality"
	ErrorInputTooLarge       ErrorCode = "input_too_large"
	ErrorUnknownModel        ErrorCode = "unknown_model"
	ErrorNoAvailableModel    ErrorCode = "no_available_model"
	ErrorProviderAuthFailed  ErrorCode = "provider_auth_failed"
	ErrorProviderRateLimited ErrorCode = "provider_rate_limited"
	ErrorProviderExhausted   ErrorCode = "provider_exhausted"
	ErrorUpstream            ErrorCode = "upstream_error"
	ErrorStreamInterrupted   ErrorCode = "stream_interrupted"
	ErrorPolicyDenied        ErrorCode = "policy_denied"
)

// ProxyError is the small internal error type shared by API handlers.
type ProxyError struct {
	Message string
	Code    ErrorCode
	Param   string
}

// Error implements the error interface.
func (e *ProxyError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// StatusCode maps the internal error code to an HTTP response status.
func (e *ProxyError) StatusCode() int {
	if e == nil {
		return http.StatusInternalServerError
	}
	return StatusForErrorCode(e.Code)
}

// StatusForErrorCode maps known internal error codes to HTTP status codes.
func StatusForErrorCode(code ErrorCode) int {
	switch code {
	case ErrorInvalidRequest:
		return http.StatusBadRequest
	case ErrorUnknownModel:
		return http.StatusNotFound
	case ErrorNoAvailableModel:
		return http.StatusServiceUnavailable
	case ErrorProviderAuthFailed:
		return http.StatusBadGateway
	case ErrorProviderRateLimited:
		return http.StatusTooManyRequests
	case ErrorProviderExhausted:
		return http.StatusServiceUnavailable
	case ErrorUpstream:
		return http.StatusBadGateway
	case ErrorStreamInterrupted:
		return http.StatusBadGateway
	case ErrorPolicyDenied:
		return http.StatusForbidden
	case ErrorUnsupportedModality:
		return http.StatusUnprocessableEntity
	case ErrorInputTooLarge:
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusInternalServerError
	}
}

// OpenAIErrorResponse is the top-level OpenAI-compatible error envelope.
type OpenAIErrorResponse struct {
	Error OpenAIError `json:"error"`
}

// OpenAIError is the error object shape expected by OpenAI-compatible clients.
type OpenAIError struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    string  `json:"code"`
}

// OpenAIErrorType maps internal codes to OpenAI-style error type strings.
func OpenAIErrorType(code ErrorCode) string {
	switch code {
	case ErrorNoAvailableModel, ErrorUnknownModel:
		return "proxy_routing_error"
	case ErrorProviderAuthFailed, ErrorProviderRateLimited, ErrorProviderExhausted, ErrorUpstream, ErrorStreamInterrupted:
		return "proxy_upstream_error"
	case ErrorPolicyDenied:
		return "policy_denied"
	default:
		return "invalid_request_error"
	}
}

// NewProxyError creates a reusable internal error for API handlers.
func NewProxyError(code ErrorCode, message string, param string) *ProxyError {
	return &ProxyError{
		Message: message,
		Code:    code,
		Param:   param,
	}
}

// WriteOpenAIError writes an OpenAI-compatible error response.
func WriteOpenAIError(w http.ResponseWriter, err *ProxyError) {
	if err == nil {
		err = NewProxyError(ErrorUpstream, "internal proxy error", "")
	}

	var param *string
	if err.Param != "" {
		param = &err.Param
	}

	writeJSON(w, err.StatusCode(), OpenAIErrorResponse{
		Error: OpenAIError{
			Message: err.Message,
			Type:    OpenAIErrorType(err.Code),
			Param:   param,
			Code:    string(err.Code),
		},
	})
}
