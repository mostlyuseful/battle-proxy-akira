package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"time"

	"battle-proxy-akira/internal/ir"
	requestlog "battle-proxy-akira/internal/logging"
	openaiapi "battle-proxy-akira/internal/openai"
	"battle-proxy-akira/internal/router"
	"battle-proxy-akira/internal/sse"
)

// RegisterChatRoutes wires Chat Completions endpoints.
func RegisterChatRoutes(mux *http.ServeMux, chatRouter router.Router, clientAuth Middleware, logger requestlog.Logger) {
	if clientAuth == nil {
		clientAuth = identityMiddleware
	}
	if logger == nil {
		logger = requestlog.NoopLogger{}
	}

	mux.Handle("POST /v1/chat/completions", clientAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		logRec := requestlog.RequestLogRecord{
			Timestamp:  started.UTC(),
			RequestID:  requestIDFromRequest(r),
			RetryCount: 0,
		}
		if chatRouter == nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorNoAvailableModel, "no chat completion router configured", "model"))
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorInvalidRequest, "read request body failed", ""))
			return
		}
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

		candidates, err := chatRouter.Resolve(r.Context(), irReq)
		if err != nil {
			writeLoggedOpenAIError(w, r, logger, logRec, started, proxyErrorFromRouterError(err))
			return
		}
		if len(candidates) == 0 {
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorNoAvailableModel, "no available provider for model", "model"))
			return
		}

		candidate := candidates[0]
		logRec.ResolvedProvider = candidate.ProviderName
		logRec.ResolvedModel = candidate.ProviderModel
		if chatReq.Stream {
			streamChatCompletion(w, r, chatRouter, candidate, irReq, logger, logRec, started)
			return
		}

		providerResp, err := candidate.Provider.Complete(r.Context(), candidate.ProviderRequest(irReq))
		if err != nil {
			chatRouter.MarkFailure(candidate, err)
			writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorUpstream, "upstream provider request failed", ""))
			return
		}
		chatRouter.MarkSuccess(candidate)

		resp := candidate.RewriteResponse(*providerResp)
		writeJSON(w, http.StatusOK, openaiapi.ChatCompletionResponseFromIR(resp, time.Now()))
		logRec.Status = http.StatusOK
		logRec.LatencyMS = time.Since(started).Milliseconds()
		_ = logger.LogRequest(r.Context(), logRec)
	})))
}

func streamChatCompletion(w http.ResponseWriter, r *http.Request, chatRouter router.Router, candidate router.RouteCandidate, irReq ir.Request, logger requestlog.Logger, logRec requestlog.RequestLogRecord, started time.Time) {
	events, err := candidate.Provider.Stream(r.Context(), candidate.ProviderRequest(irReq))
	if err != nil {
		chatRouter.MarkFailure(candidate, err)
		writeLoggedOpenAIError(w, r, logger, logRec, started, NewProxyError(ErrorUpstream, "upstream provider stream failed", ""))
		return
	}

	sse.SetHeaders(w.Header())
	w.WriteHeader(http.StatusOK)
	for event := range events {
		if err := sse.WriteData(w, event.Text); err != nil {
			chatRouter.MarkFailure(candidate, err)
			logRec.Status = http.StatusOK
			logRec.LatencyMS = time.Since(started).Milliseconds()
			_ = logger.LogRequest(r.Context(), logRec)
			return
		}
	}
	chatRouter.MarkSuccess(candidate)
	logRec.Status = http.StatusOK
	logRec.LatencyMS = time.Since(started).Milliseconds()
	_ = logger.LogRequest(r.Context(), logRec)
}

func writeLoggedOpenAIError(w http.ResponseWriter, r *http.Request, logger requestlog.Logger, rec requestlog.RequestLogRecord, started time.Time, proxyErr *ProxyError) {
	WriteOpenAIError(w, proxyErr)
	if proxyErr == nil {
		proxyErr = NewProxyError(ErrorUpstream, "internal proxy error", "")
	}
	rec.Status = proxyErr.StatusCode()
	rec.LatencyMS = time.Since(started).Milliseconds()
	_ = logger.LogRequest(r.Context(), rec)
}

func requestIDFromRequest(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return "req_" + hex.EncodeToString(b[:])
}

func proxyErrorFromRouterError(err error) *ProxyError {
	var routerErr *router.Error
	if errors.As(err, &routerErr) {
		switch routerErr.Code {
		case router.ErrorUnknownModel:
			return NewProxyError(ErrorUnknownModel, routerErr.Message, routerErr.Param)
		case router.ErrorNoAvailableModel:
			return NewProxyError(ErrorNoAvailableModel, routerErr.Message, routerErr.Param)
		}
		return NewProxyError(ErrorNoAvailableModel, routerErr.Message, routerErr.Param)
	}
	return NewProxyError(ErrorUpstream, err.Error(), "")
}
