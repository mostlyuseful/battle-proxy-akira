package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"battle-proxy-akira/internal/ir"
	requestlog "battle-proxy-akira/internal/logging"
	openaiapi "battle-proxy-akira/internal/openai"
	providerpkg "battle-proxy-akira/internal/provider"
	"battle-proxy-akira/internal/router"
	"battle-proxy-akira/internal/sse"
)

// RegisterChatRoutes wires Chat Completions endpoints.
func RegisterChatRoutes(mux *http.ServeMux, chatRouter router.Router, clientAuth Middleware, logger requestlog.Logger, maxBodyBytes int64) {
	if clientAuth == nil {
		clientAuth = identityMiddleware
	}
	if logger == nil {
		logger = requestlog.NoopLogger{}
	}

	mux.Handle("POST /v1/chat/completions", clientAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := requestIDForRequest(r)
		r = r.WithContext(ContextWithRequestID(r.Context(), requestID))
		logRec := newRequestLogRecord(r, "chat_completions", requestID)
		logRec.Timestamp = started.UTC()
		if chatRouter == nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorNoAvailableModel, "no chat completion router configured", "model"))
			return
		}

		body, err := readLimitedBody(w, r, maxBodyBytes)
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, proxyErrorFromReadBodyError(err))
			return
		}
		attachRequestTranscript(logger, &logRec, body)
		chatReq, err := openaiapi.ParseChatCompletionRequest(body)
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorInvalidRequest, "invalid Chat Completions request JSON", ""))
			return
		}
		logRec.RequestedModel = chatReq.Model
		logRec.Stream = chatReq.Stream
		irReq, err := chatReq.ToIR()
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorInvalidRequest, err.Error(), ""))
			return
		}
		irReq.ID = requestID
		if irReq.Metadata == nil {
			irReq.Metadata = map[string]string{}
		}
		irReq.Metadata["request_id"] = requestID
		logRec.ImageInputs = requestlog.ImageMetadataFromRequest(irReq)

		candidates, err := chatRouter.Resolve(r.Context(), irReq)
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, proxyErrorFromRouterError(err))
			return
		}
		if len(candidates) == 0 {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorNoAvailableModel, "no available provider for model", "model"))
			return
		}

		if chatReq.Stream {
			streamChatCompletion(w, r, chatRouter, candidates, irReq, logger, logRec, started)
			return
		}

		completeChatCompletion(w, r, chatRouter, candidates, irReq, logger, logRec, started)
	})))
}

func completeChatCompletion(w http.ResponseWriter, r *http.Request, chatRouter router.Router, candidates []router.RouteCandidate, irReq ir.Request, logger requestlog.Logger, logRec requestlog.RequestLogRecord, started time.Time) {
	retryCount := 0
	for i, candidate := range candidates {
		attemptLog := logRec
		attemptLog.ResolvedProvider = candidate.ProviderName
		attemptLog.ResolvedModel = candidate.ProviderModel
		attemptLog.RetryCount = retryCount
		transcriptAttempt := appendTranscriptAttempt(&attemptLog, candidate.ProviderName, candidate.ProviderModel)

		providerResp, err := candidate.Provider.Complete(r.Context(), candidate.ProviderRequest(irReq))
		if err != nil {
			if transcriptAttempt != nil {
				transcriptAttempt.Error = err.Error()
			}
			chatRouter.MarkFailure(candidate, err)
			if providerpkg.IsRetryable(err) && i+1 < len(candidates) {
				retryCount++
				continue
			}
			writeLoggedOpenAIError(w, r, logger, attemptLog, started, proxyErrorFromProviderError(err, "upstream provider request failed"))
			return
		}
		chatRouter.MarkSuccess(candidate)
		if transcriptAttempt != nil {
			transcriptAttempt.Response = append(json.RawMessage(nil), providerResp.RawBody...)
		}

		resp := candidate.RewriteResponse(*providerResp)
		writeJSON(w, http.StatusOK, openaiapi.ChatCompletionResponseFromIR(resp, time.Now()))
		attemptLog.Status = http.StatusOK
		attemptLog.LatencyMS = time.Since(started).Milliseconds()
		_ = logger.LogRequest(r.Context(), attemptLog)
		return
	}
}

func readLimitedBody(w http.ResponseWriter, r *http.Request, maxBodyBytes int64) ([]byte, error) {
	if maxBodyBytes > 0 && r.ContentLength > maxBodyBytes {
		return nil, &http.MaxBytesError{Limit: maxBodyBytes}
	}
	reader := r.Body
	if maxBodyBytes > 0 {
		reader = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func proxyErrorFromReadBodyError(err error) *ProxyError {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return NewProxyError(ErrorInputTooLarge, "request body exceeds maximum size", "")
	}
	return NewProxyError(ErrorInvalidRequest, "read request body failed", "")
}

func streamChatCompletion(w http.ResponseWriter, r *http.Request, chatRouter router.Router, candidates []router.RouteCandidate, irReq ir.Request, logger requestlog.Logger, logRec requestlog.RequestLogRecord, started time.Time) {
	retryCount := 0
candidateLoop:
	for i, candidate := range candidates {
		attemptLog := logRec
		attemptLog.ResolvedProvider = candidate.ProviderName
		attemptLog.ResolvedModel = candidate.ProviderModel
		attemptLog.RetryCount = retryCount
		transcriptAttempt := appendTranscriptAttempt(&attemptLog, candidate.ProviderName, candidate.ProviderModel)

		events, err := candidate.Provider.Stream(r.Context(), candidate.ProviderRequest(irReq))
		if err != nil {
			if transcriptAttempt != nil {
				transcriptAttempt.Error = err.Error()
			}
			chatRouter.MarkFailure(candidate, err)
			if providerpkg.IsRetryable(err) && i+1 < len(candidates) {
				retryCount++
				continue
			}
			writeLoggedOpenAIError(w, r, logger, attemptLog, started, proxyErrorFromProviderError(err, "upstream provider stream failed"))
			return
		}

		emitted := false
		for event := range events {
			if transcriptAttempt != nil {
				if len(event.Raw) > 0 {
					transcriptAttempt.Stream = append(transcriptAttempt.Stream, append(json.RawMessage(nil), event.Raw...))
				} else if event.Text != "" {
					transcriptAttempt.Stream = append(transcriptAttempt.Stream, json.RawMessage(strconv.Quote(event.Text)))
				}
			}
			if event.Type == ir.EventTypeError || event.Error != nil {
				err := errors.New("upstream provider stream interrupted")
				if transcriptAttempt != nil {
					transcriptAttempt.Error = err.Error()
				}
				chatRouter.MarkFailure(candidate, err)
				if !emitted {
					if isRetryableStreamEvent(event) && i+1 < len(candidates) {
						retryCount++
						continue candidateLoop
					}
					writeLoggedOpenAIError(w, r, logger, attemptLog, started, NewProxyError(ErrorStreamInterrupted, "upstream provider stream interrupted", ""))
					return
				}
				_ = writeStreamInterruptedEvent(w)
				attemptLog.Status = http.StatusOK
				attemptLog.LatencyMS = time.Since(started).Milliseconds()
				_ = logger.LogRequest(r.Context(), attemptLog)
				return
			}
			if !emitted {
				sse.SetHeaders(w.Header())
				w.WriteHeader(http.StatusOK)
				emitted = true
			}
			if err := sse.WriteData(w, event.Text); err != nil {
				chatRouter.MarkFailure(candidate, err)
				attemptLog.Status = http.StatusOK
				attemptLog.LatencyMS = time.Since(started).Milliseconds()
				_ = logger.LogRequest(r.Context(), attemptLog)
				return
			}
		}
		if !emitted {
			sse.SetHeaders(w.Header())
			w.WriteHeader(http.StatusOK)
		}
		chatRouter.MarkSuccess(candidate)
		attemptLog.Status = http.StatusOK
		attemptLog.LatencyMS = time.Since(started).Milliseconds()
		_ = logger.LogRequest(r.Context(), attemptLog)
		return
	}
}

func isRetryableStreamEvent(event ir.Event) bool {
	if event.Error == nil {
		return false
	}
	switch event.Error.Code {
	case providerpkg.ErrorProviderRateLimited, providerpkg.ErrorUpstream:
		return true
	default:
		return false
	}
}

func writeStreamInterruptedEvent(w http.ResponseWriter) error {
	encoded, err := json.Marshal(OpenAIErrorResponse{Error: OpenAIError{Message: "upstream provider stream interrupted", Type: OpenAIErrorType(ErrorStreamInterrupted), Code: string(ErrorStreamInterrupted)}})
	if err != nil {
		return err
	}
	return sse.WriteData(w, string(encoded))
}

func writeLoggedOpenAIError(w http.ResponseWriter, r *http.Request, logger requestlog.Logger, rec requestlog.RequestLogRecord, started time.Time, proxyErr *ProxyError) {
	WriteOpenAIError(w, proxyErr)
	if proxyErr == nil {
		proxyErr = NewProxyError(ErrorUpstream, "internal proxy error", "")
	}
	recordErrorFromProxy(r.Context(), proxyErr.Code)
	rec.Status = proxyErr.StatusCode()
	rec.LatencyMS = time.Since(started).Milliseconds()
	_ = logger.LogRequest(r.Context(), rec)
}

func proxyErrorFromProviderError(err error, fallbackMessage string) *ProxyError {
	var providerErr *providerpkg.Error
	if errors.As(err, &providerErr) {
		switch providerErr.Code {
		case providerpkg.ErrorInvalidRequest:
			return NewProxyError(ErrorInvalidRequest, "upstream provider rejected the request", "")
		case providerpkg.ErrorUnsupportedModality:
			return NewProxyError(ErrorUnsupportedModality, "upstream provider does not support the requested modality", "")
		case providerpkg.ErrorInputTooLarge:
			return NewProxyError(ErrorInputTooLarge, "upstream provider rejected the request as too large", "")
		case providerpkg.ErrorProviderAuthFailed:
			return NewProxyError(ErrorProviderAuthFailed, "upstream provider authentication failed", "")
		case providerpkg.ErrorProviderRateLimited:
			return NewProxyError(ErrorProviderRateLimited, "upstream provider rate limited the request", "")
		case providerpkg.ErrorProviderExhausted:
			return NewProxyError(ErrorProviderExhausted, "upstream provider is exhausted", "")
		case providerpkg.ErrorPolicyDenied:
			return NewProxyError(ErrorPolicyDenied, "upstream provider denied the request", "")
		}
	}
	return NewProxyError(ErrorUpstream, fallbackMessage, "")
}

func proxyErrorFromRouterError(err error) *ProxyError {
	var routerErr *router.Error
	if errors.As(err, &routerErr) {
		switch routerErr.Code {
		case router.ErrorUnknownModel:
			return NewProxyError(ErrorUnknownModel, routerErr.Message, routerErr.Param)
		case router.ErrorNoAvailableModel:
			return NewProxyError(ErrorNoAvailableModel, routerErr.Message, routerErr.Param)
		case router.ErrorUnsupportedModality:
			return NewProxyError(ErrorUnsupportedModality, routerErr.Message, routerErr.Param)
		}
		return NewProxyError(ErrorNoAvailableModel, routerErr.Message, routerErr.Param)
	}
	return NewProxyError(ErrorUpstream, err.Error(), "")
}
