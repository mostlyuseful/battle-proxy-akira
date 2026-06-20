package api

import (
	"net/http"
	"time"

	"battle-proxy-akira/internal/ir"
	requestlog "battle-proxy-akira/internal/logging"
	openaiapi "battle-proxy-akira/internal/openai"
	providerpkg "battle-proxy-akira/internal/provider"
	"battle-proxy-akira/internal/router"
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
		logRec := requestlog.RequestLogRecord{
			Timestamp:  started.UTC(),
			RequestID:  requestID,
			RetryCount: 0,
		}
		if responsesRouter == nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorNoAvailableModel, "no responses router configured", "model"))
			return
		}

		body, err := readLimitedBody(w, r, maxBodyBytes)
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, proxyErrorFromReadBodyError(err))
			return
		}
		respReq, err := openaiapi.ParseResponseRequest(body)
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorInvalidRequest, "invalid Responses request JSON", ""))
			return
		}
		logRec.RequestedModel = respReq.Model
		logRec.Stream = respReq.Stream

		// Streaming Responses is a separate task; fail clearly rather than
		// silently degrading to a non-streaming response.
		if respReq.Stream {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorInvalidRequest, "streaming Responses are not supported yet", "stream"))
			return
		}

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

		providerResp, err := candidate.Provider.Complete(r.Context(), candidate.ProviderRequest(irReq))
		if err != nil {
			responsesRouter.MarkFailure(candidate, err)
			if providerpkg.IsRetryable(err) && i+1 < len(candidates) {
				retryCount++
				continue
			}
			writeLoggedOpenAIError(w, r, logger, attemptLog, started, proxyErrorFromProviderError(err, "upstream provider request failed"))
			return
		}
		responsesRouter.MarkSuccess(candidate)

		resp := candidate.RewriteResponse(*providerResp)
		writeJSON(w, http.StatusOK, openaiapi.ResponseFromIR(resp, time.Now()))
		attemptLog.Status = http.StatusOK
		attemptLog.LatencyMS = time.Since(started).Milliseconds()
		_ = logger.LogRequest(r.Context(), attemptLog)
		return
	}
}
