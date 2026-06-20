package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"battle-proxy-akira/internal/ir"
	requestlog "battle-proxy-akira/internal/logging"
	openaiapi "battle-proxy-akira/internal/openai"
	providerpkg "battle-proxy-akira/internal/provider"
	"battle-proxy-akira/internal/router"
	"battle-proxy-akira/internal/sse"
)

// RegisterResponsesRoutes wires the non-streaming Responses API endpoint.
func RegisterResponsesRoutes(mux *http.ServeMux, responsesRouter router.Router, clientAuth Middleware, logger requestlog.Logger, maxBodyBytes int64) {
	if clientAuth == nil {
		clientAuth = identityMiddleware
	}
	if logger == nil {
		logger = requestlog.NoopLogger{}
	}

	mux.Handle("POST /v1/responses", clientAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := requestIDForRequest(r)
		r = r.WithContext(ContextWithRequestID(r.Context(), requestID))
		logRec := newRequestLogRecord(r, "responses", requestID)
		logRec.Timestamp = started.UTC()
		if responsesRouter == nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorNoAvailableModel, "no responses router configured", "model"))
			return
		}

		body, err := readLimitedBody(w, r, maxBodyBytes)
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, proxyErrorFromReadBodyError(err))
			return
		}
		attachRequestTranscript(logger, &logRec, body)
		respReq, err := openaiapi.ParseResponseRequest(body)
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorInvalidRequest, "invalid Responses request JSON", ""))
			return
		}
		logRec.RequestedModel = respReq.Model
		logRec.Stream = respReq.Stream

		irReq, err := respReq.ToIR()
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorInvalidRequest, err.Error(), ""))
			return
		}
		irReq.ID = requestID
		if irReq.Metadata == nil {
			irReq.Metadata = map[string]string{}
		}
		irReq.Metadata["request_id"] = requestID
		irReq.Metadata["api"] = "responses"
		logRec.ImageInputs = requestlog.ImageMetadataFromRequest(irReq)

		candidates, err := responsesRouter.Resolve(r.Context(), irReq)
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, proxyErrorFromRouterError(err))
			return
		}
		if len(candidates) == 0 {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorNoAvailableModel, "no available provider for model", "model"))
			return
		}

		if respReq.Stream {
			streamResponsesRequest(w, r, responsesRouter, candidates, irReq, logger, logRec, started)
			return
		}

		completeResponsesRequest(w, r, responsesRouter, candidates, irReq, logger, logRec, started)
	})))
}

func completeResponsesRequest(w http.ResponseWriter, r *http.Request, responsesRouter router.Router, candidates []router.RouteCandidate, irReq ir.Request, logger requestlog.Logger, logRec requestlog.RequestLogRecord, started time.Time) {
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
			responsesRouter.MarkFailure(candidate, err)
			if providerpkg.IsRetryable(err) && i+1 < len(candidates) {
				retryCount++
				continue
			}
			writeLoggedOpenAIError(w, r, logger, attemptLog, started, proxyErrorFromProviderError(err, "upstream provider request failed"))
			return
		}
		responsesRouter.MarkSuccess(candidate)
		if transcriptAttempt != nil {
			transcriptAttempt.Response = append(json.RawMessage(nil), providerResp.RawBody...)
		}

		resp := candidate.RewriteResponse(*providerResp)
		writeJSON(w, http.StatusOK, openaiapi.ResponseFromIR(resp, time.Now()))
		attemptLog.Status = http.StatusOK
		attemptLog.LatencyMS = time.Since(started).Milliseconds()
		_ = logger.LogRequest(r.Context(), attemptLog)
		return
	}
}

func streamResponsesRequest(w http.ResponseWriter, r *http.Request, responsesRouter router.Router, candidates []router.RouteCandidate, irReq ir.Request, logger requestlog.Logger, logRec requestlog.RequestLogRecord, started time.Time) {
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
			responsesRouter.MarkFailure(candidate, err)
			if providerpkg.IsRetryable(err) && i+1 < len(candidates) {
				retryCount++
				continue
			}
			writeLoggedOpenAIError(w, r, logger, attemptLog, started, proxyErrorFromProviderError(err, "upstream provider stream failed"))
			return
		}

		responseID := responsesResponseID(irReq.ID)
		itemID := responsesItemID(irReq.ID)
		translator := openaiapi.NewResponsesStreamTranslator(responseID, itemID, candidate.RequestedModel, started.Unix())

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
				responsesRouter.MarkFailure(candidate, responsesStreamError(event))
				if transcriptAttempt != nil {
					transcriptAttempt.Error = responsesStreamError(event).Error()
				}
				if !emitted {
					if isRetryableStreamEvent(event) && i+1 < len(candidates) {
						retryCount++
						continue candidateLoop
					}
					writeLoggedOpenAIError(w, r, logger, attemptLog, started, NewProxyError(ErrorStreamInterrupted, "upstream provider stream interrupted", ""))
					return
				}
				// Mid-stream failure: emit an error SSE event then close,
				// per the project streaming policy. Never fall back after
				// the first event has been sent.
				code := providerpkg.ErrorCode(responsesStreamError(event))
				if code == "" {
					code = providerpkg.ErrorUpstream
				}
				_ = translator.WriteError(w, code, "upstream provider stream interrupted")
				attemptLog.Status = http.StatusOK
				attemptLog.LatencyMS = time.Since(started).Milliseconds()
				_ = logger.LogRequest(r.Context(), attemptLog)
				return
			}
			if !emitted {
				sse.SetHeaders(w.Header())
				w.WriteHeader(http.StatusOK)
				emitted = true
				if err := translator.WriteOpening(w); err != nil {
					responsesRouter.MarkFailure(candidate, err)
					attemptLog.Status = http.StatusOK
					attemptLog.LatencyMS = time.Since(started).Milliseconds()
					_ = logger.LogRequest(r.Context(), attemptLog)
					return
				}
			}
			if err := translator.Translate(w, event); err != nil {
				responsesRouter.MarkFailure(candidate, err)
				attemptLog.Status = http.StatusOK
				attemptLog.LatencyMS = time.Since(started).Milliseconds()
				_ = logger.LogRequest(r.Context(), attemptLog)
				return
			}
		}
		if !emitted {
			sse.SetHeaders(w.Header())
			w.WriteHeader(http.StatusOK)
			emitted = true
			if err := translator.WriteOpening(w); err != nil {
				responsesRouter.MarkFailure(candidate, err)
				attemptLog.Status = http.StatusOK
				attemptLog.LatencyMS = time.Since(started).Milliseconds()
				_ = logger.LogRequest(r.Context(), attemptLog)
				return
			}
		}
		if err := translator.WriteClosing(w); err != nil {
			responsesRouter.MarkFailure(candidate, err)
			attemptLog.Status = http.StatusOK
			attemptLog.LatencyMS = time.Since(started).Milliseconds()
			_ = logger.LogRequest(r.Context(), attemptLog)
			return
		}
		responsesRouter.MarkSuccess(candidate)
		attemptLog.Status = http.StatusOK
		attemptLog.LatencyMS = time.Since(started).Milliseconds()
		_ = logger.LogRequest(r.Context(), attemptLog)
		return
	}
}

// responsesStreamError returns a non-nil error describing a stream event failure.
func responsesStreamError(event ir.Event) error {
	if event.Error != nil {
		return &providerpkg.Error{Code: event.Error.Code, Provider: event.Model}
	}
	return &providerpkg.Error{Code: providerpkg.ErrorUpstream, Provider: event.Model}
}

// responsesResponseID derives a Responses response ID from the request ID.
func responsesResponseID(requestID string) string {
	if requestID == "" {
		return "resp_stream"
	}
	if rest, ok := strings.CutPrefix(requestID, "req_"); ok {
		return "resp_" + rest
	}
	return "resp_" + requestID
}

// responsesItemID derives a Responses output item ID from the request ID.
func responsesItemID(requestID string) string {
	if requestID == "" {
		return "msg_stream"
	}
	if rest, ok := strings.CutPrefix(requestID, "req_"); ok {
		return "msg_" + rest
	}
	return "msg_" + requestID
}
