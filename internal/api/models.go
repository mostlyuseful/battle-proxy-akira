package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"

	"battle-proxy-akira/internal/ir"
)

// ModelLister is the router-facing subset needed by GET /v1/models.
type ModelLister interface {
	Models(ctx context.Context) ([]ir.Model, error)
}

// ModelListerFunc adapts a function into a ModelLister.
type ModelListerFunc func(ctx context.Context) ([]ir.Model, error)

// Models implements ModelLister.
func (f ModelListerFunc) Models(ctx context.Context) ([]ir.Model, error) {
	return f(ctx)
}

type modelListResponse struct {
	Object string          `json:"object"`
	Data   []modelResponse `json:"data"`
}

type modelResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// RegisterModelRoutes wires model metadata endpoints.
func RegisterModelRoutes(mux *http.ServeMux, lister ModelLister, clientAuth Middleware, logger *slog.Logger) {
	if lister == nil {
		lister = ModelListerFunc(emptyModels)
	}
	if clientAuth == nil {
		clientAuth = identityMiddleware
	}

	mux.Handle("GET /v1/models", clientAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := requestIDForRequest(r)
		logRec := newRequestLogRecord(r, "models", requestID)
		logRequestAccepted(logger, r, requestID, logRec.Endpoint)
		logRequestStarted(logger, logRec)
		models, err := lister.Models(r.Context())
		if err != nil {
			WriteOpenAIError(w, NewProxyError(ErrorUpstream, "list models failed", ""))
			logRec.Status = http.StatusBadGateway
			logRequestFinished(logger, logRec)
			return
		}

		writeJSON(w, http.StatusOK, modelListResponse{
			Object: "list",
			Data:   openAIModelResponses(models),
		})
		logRec.Status = http.StatusOK
		logRequestFinished(logger, logRec)
	})))
}

func openAIModelResponses(models []ir.Model) []modelResponse {
	out := make([]modelResponse, 0, len(models))
	for _, model := range models {
		owner := model.Provider
		if owner == "" || model.Synthetic {
			owner = "proxy"
		}
		id := model.ID
		if !model.Synthetic && owner != "" && owner != "proxy" {
			id = owner + ":" + model.ID
		}
		out = append(out, modelResponse{
			ID:      id,
			Object:  "model",
			Created: 0,
			OwnedBy: owner,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func emptyModels(context.Context) ([]ir.Model, error) {
	return nil, nil
}

func identityMiddleware(next http.Handler) http.Handler {
	return next
}
