package api

import (
	"errors"
	"io"
	"net/http"
	"time"

	"battle-proxy-akira/internal/ir"
	openaiapi "battle-proxy-akira/internal/openai"
	"battle-proxy-akira/internal/router"
	"battle-proxy-akira/internal/sse"
)

// RegisterChatRoutes wires Chat Completions endpoints.
func RegisterChatRoutes(mux *http.ServeMux, chatRouter router.Router, clientAuth Middleware) {
	if clientAuth == nil {
		clientAuth = identityMiddleware
	}

	mux.Handle("POST /v1/chat/completions", clientAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if chatRouter == nil {
			WriteOpenAIError(w, NewProxyError(ErrorNoAvailableModel, "no chat completion router configured", "model"))
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			WriteOpenAIError(w, NewProxyError(ErrorInvalidRequest, "read request body failed", ""))
			return
		}
		chatReq, err := openaiapi.ParseChatCompletionRequest(body)
		if err != nil {
			WriteOpenAIError(w, NewProxyError(ErrorInvalidRequest, "invalid Chat Completions request JSON", ""))
			return
		}
		irReq, err := chatReq.ToIR()
		if err != nil {
			WriteOpenAIError(w, NewProxyError(ErrorInvalidRequest, err.Error(), ""))
			return
		}

		candidates, err := chatRouter.Resolve(r.Context(), irReq)
		if err != nil {
			WriteOpenAIError(w, proxyErrorFromRouterError(err))
			return
		}
		if len(candidates) == 0 {
			WriteOpenAIError(w, NewProxyError(ErrorNoAvailableModel, "no available provider for model", "model"))
			return
		}

		candidate := candidates[0]
		if chatReq.Stream {
			streamChatCompletion(w, r, chatRouter, candidate, irReq)
			return
		}

		providerResp, err := candidate.Provider.Complete(r.Context(), candidate.ProviderRequest(irReq))
		if err != nil {
			chatRouter.MarkFailure(candidate, err)
			WriteOpenAIError(w, NewProxyError(ErrorUpstream, "upstream provider request failed", ""))
			return
		}
		chatRouter.MarkSuccess(candidate)

		resp := candidate.RewriteResponse(*providerResp)
		writeJSON(w, http.StatusOK, openaiapi.ChatCompletionResponseFromIR(resp, time.Now()))
	})))
}

func streamChatCompletion(w http.ResponseWriter, r *http.Request, chatRouter router.Router, candidate router.RouteCandidate, irReq ir.Request) {
	events, err := candidate.Provider.Stream(r.Context(), candidate.ProviderRequest(irReq))
	if err != nil {
		chatRouter.MarkFailure(candidate, err)
		WriteOpenAIError(w, NewProxyError(ErrorUpstream, "upstream provider stream failed", ""))
		return
	}

	sse.SetHeaders(w.Header())
	w.WriteHeader(http.StatusOK)
	for event := range events {
		if err := sse.WriteData(w, event.Text); err != nil {
			chatRouter.MarkFailure(candidate, err)
			return
		}
	}
	chatRouter.MarkSuccess(candidate)
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
