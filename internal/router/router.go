// Package router resolves requested model names to provider route candidates.
package router

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"battle-proxy-akira/internal/config"
	"battle-proxy-akira/internal/ir"
	"battle-proxy-akira/internal/provider"
)

const (
	// ErrorUnknownModel means the requested model is not configured.
	ErrorUnknownModel = "unknown_model"
	// ErrorNoAvailableModel means the model exists in config but cannot be routed to an available provider.
	ErrorNoAvailableModel = "no_available_model"
)

// Error describes a routing failure with a stable internal code.
type Error struct {
	Code    string
	Message string
	Param   string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// RouteCandidate is one provider/model target for a normalized request.
type RouteCandidate struct {
	ProviderName   string
	ProviderModel  string
	RequestedModel string
	Provider       provider.Provider
}

// Router resolves requests to route candidates and records route outcomes.
type Router interface {
	Resolve(ctx context.Context, req ir.Request) ([]RouteCandidate, error)
	MarkFailure(candidate RouteCandidate, err error)
	MarkSuccess(candidate RouteCandidate)
}

// StaticRouter resolves only direct configured provider/model pairs.
type StaticRouter struct {
	cfg       config.Config
	providers map[string]provider.Provider
}

// NewStatic creates a deterministic router for configured direct models.
func NewStatic(cfg config.Config, providers map[string]provider.Provider) *StaticRouter {
	if providers == nil {
		providers = map[string]provider.Provider{}
	}
	return &StaticRouter{cfg: cfg, providers: providers}
}

// Resolve returns a single direct route candidate for req.Model.
func (r *StaticRouter) Resolve(ctx context.Context, req ir.Request) ([]RouteCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return nil, &Error{Code: ErrorUnknownModel, Message: "model is required", Param: "model"}
	}

	if providerName, providerModel, ok := strings.Cut(model, ":"); ok {
		return r.resolveProviderModel(providerName, providerModel, model)
	}
	return r.resolveDirectModel(model)
}

// MarkFailure is a no-op for the static router. Later routers add fallback and circuit state.
func (r *StaticRouter) MarkFailure(candidate RouteCandidate, err error) {}

// MarkSuccess is a no-op for the static router. Later routers add fallback and circuit state.
func (r *StaticRouter) MarkSuccess(candidate RouteCandidate) {}

func (r *StaticRouter) resolveProviderModel(providerName, providerModel, requestedModel string) ([]RouteCandidate, error) {
	providerName = strings.TrimSpace(providerName)
	providerModel = strings.TrimSpace(providerModel)
	if providerName == "" || providerModel == "" {
		return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("unknown model %q", requestedModel), Param: "model"}
	}
	providerCfg, ok := r.cfg.Providers[providerName]
	if !ok {
		return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("unknown provider %q for model %q", providerName, requestedModel), Param: "model"}
	}
	if _, ok := providerCfg.Models[providerModel]; !ok {
		return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("unknown model %q for provider %q", providerModel, providerName), Param: "model"}
	}
	return r.candidate(providerName, providerModel, requestedModel)
}

func (r *StaticRouter) resolveDirectModel(model string) ([]RouteCandidate, error) {
	providerNames := sortedProviderNames(r.cfg.Providers)
	for _, providerName := range providerNames {
		providerCfg := r.cfg.Providers[providerName]
		if _, ok := providerCfg.Models[model]; ok {
			return r.candidate(providerName, model, model)
		}
	}
	return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("unknown model %q", model), Param: "model"}
}

func (r *StaticRouter) candidate(providerName, providerModel, requestedModel string) ([]RouteCandidate, error) {
	p, ok := r.providers[providerName]
	if !ok || p == nil {
		return nil, &Error{Code: ErrorNoAvailableModel, Message: fmt.Sprintf("no available provider for model %q", requestedModel), Param: "model"}
	}
	return []RouteCandidate{
		{
			ProviderName:   providerName,
			ProviderModel:  providerModel,
			RequestedModel: requestedModel,
			Provider:       p,
		},
	}, nil
}

func sortedProviderNames(providers map[string]config.ProviderConfig) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
