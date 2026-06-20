// Package router resolves requested model names to provider route candidates.
package router

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"battle-proxy-akira/internal/config"
	"battle-proxy-akira/internal/ir"
	"battle-proxy-akira/internal/provider"
)

const (
	// ErrorUnknownModel means the requested model is not configured.
	ErrorUnknownModel = "unknown_model"
	// ErrorNoAvailableModel means the model exists in config but cannot be routed to an available provider.
	ErrorNoAvailableModel = "no_available_model"
	// ErrorUnsupportedModality means the model exists but cannot support the request modalities.
	ErrorUnsupportedModality = "unsupported_modality"
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
	Synthetic      bool
	Provider       provider.Provider
}

// ProviderRequest returns a copy of req targeted at the concrete provider model.
func (c RouteCandidate) ProviderRequest(req ir.Request) ir.Request {
	req.Model = c.ProviderModel
	return req
}

// RewriteResponse returns a copy of resp with the client-requested model name.
func (c RouteCandidate) RewriteResponse(resp ir.Response) ir.Response {
	resp.Model = c.RequestedModel
	return resp
}

// Router resolves requests to route candidates and records route outcomes.
type Router interface {
	Resolve(ctx context.Context, req ir.Request) ([]RouteCandidate, error)
	MarkFailure(candidate RouteCandidate, err error)
	MarkSuccess(candidate RouteCandidate)
}

// StaticRouter resolves configured direct models and first_available synthetic aliases.
type StaticRouter struct {
	cfg          config.Config
	providers    map[string]provider.Provider
	availability *AvailabilityTracker
}

// NewStatic creates a deterministic router for configured direct and synthetic models.
func NewStatic(cfg config.Config, providers map[string]provider.Provider) *StaticRouter {
	if providers == nil {
		providers = map[string]provider.Provider{}
	}
	return &StaticRouter{cfg: cfg, providers: providers, availability: NewAvailabilityTracker()}
}

// Resolve returns route candidates for req.Model.
func (r *StaticRouter) Resolve(ctx context.Context, req ir.Request) ([]RouteCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return nil, &Error{Code: ErrorUnknownModel, Message: "model is required", Param: "model"}
	}

	requiredModalities := req.InputModalities()
	if providerName, providerModel, ok := strings.Cut(model, ":"); ok {
		return r.resolveProviderModel(providerName, providerModel, model, requiredModalities)
	}
	if synthetic, ok := r.cfg.SyntheticModels[model]; ok {
		return r.resolveSyntheticModel(model, synthetic, requiredModalities)
	}
	return r.resolveDirectModel(model, requiredModalities)
}

// Models returns configured direct models plus exposed synthetic aliases.
func (r *StaticRouter) Models(ctx context.Context) ([]ir.Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	modelsByID := map[string]ir.Model{}
	for _, providerName := range sortedProviderNames(r.cfg.Providers) {
		providerCfg := r.cfg.Providers[providerName]
		modelNames := sortedModelNames(providerCfg.Models)
		for _, modelName := range modelNames {
			modelCfg := providerCfg.Models[modelName]
			if _, exists := modelsByID[modelName]; exists {
				continue
			}
			modelsByID[modelName] = ir.Model{
				ID:         modelName,
				Provider:   providerName,
				Name:       modelName,
				Modalities: append([]string(nil), modelCfg.Modalities...),
			}
		}
	}

	for _, alias := range sortedSyntheticNames(r.cfg.SyntheticModels) {
		synthetic := r.cfg.SyntheticModels[alias]
		if !synthetic.Expose {
			continue
		}
		modelsByID[alias] = ir.Model{
			ID:        alias,
			Provider:  "proxy",
			Name:      alias,
			Synthetic: true,
			Metadata: map[string]string{
				"strategy": synthetic.Strategy,
			},
		}
	}

	ids := make([]string, 0, len(modelsByID))
	for id := range modelsByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	models := make([]ir.Model, 0, len(ids))
	for _, id := range ids {
		models = append(models, modelsByID[id])
	}
	return models, nil
}

// MarkFailure records provider/model availability state for the route candidate.
func (r *StaticRouter) MarkFailure(candidate RouteCandidate, err error) {
	if r.availability != nil {
		r.availability.MarkFailure(candidate, err)
	}
}

// MarkSuccess records provider/model availability state for the route candidate.
func (r *StaticRouter) MarkSuccess(candidate RouteCandidate) {
	if r.availability != nil {
		r.availability.MarkSuccess(candidate)
	}
}

// Availability returns a snapshot of one provider/model availability state.
func (r *StaticRouter) Availability(providerName, modelName string) (AvailabilityState, bool) {
	if r.availability == nil {
		return AvailabilityState{}, false
	}
	return r.availability.Get(providerName, modelName)
}

// AvailabilityStates returns snapshots for all tracked provider/model states.
func (r *StaticRouter) AvailabilityStates() []AvailabilityState {
	if r.availability == nil {
		return nil
	}
	return r.availability.States()
}

// IsCandidateAvailable reports whether a candidate is not in an active unavailable window.
func (r *StaticRouter) IsCandidateAvailable(candidate RouteCandidate, at time.Time) bool {
	if r.availability == nil {
		return true
	}
	return r.availability.IsAvailable(candidate.ProviderName, candidate.ProviderModel, at)
}

func (r *StaticRouter) resolveSyntheticModel(alias string, synthetic config.SyntheticModelConfig, requiredModalities []string) ([]RouteCandidate, error) {
	if synthetic.Strategy != config.SyntheticStrategyFirstAvailable {
		return nil, &Error{Code: ErrorNoAvailableModel, Message: fmt.Sprintf("synthetic model %q uses unsupported strategy %q", alias, synthetic.Strategy), Param: "model"}
	}

	candidates := make([]RouteCandidate, 0, len(synthetic.Candidates))
	missingProviders := 0
	unsupportedModalities := 0
	for _, candidate := range synthetic.Candidates {
		providerName, providerModel, ok := strings.Cut(candidate, ":")
		if !ok || providerName == "" || providerModel == "" {
			return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("synthetic model %q contains invalid candidate %q", alias, candidate), Param: "model"}
		}
		providerCfg, ok := r.cfg.Providers[providerName]
		if !ok {
			return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("synthetic model %q references unknown provider %q", alias, providerName), Param: "model"}
		}
		modelCfg, ok := providerCfg.Models[providerModel]
		if !ok {
			return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("synthetic model %q references unknown model %q for provider %q", alias, providerModel, providerName), Param: "model"}
		}
		if !supportsModalities(modelCfg.Modalities, requiredModalities) {
			unsupportedModalities++
			continue
		}
		p, ok := r.providers[providerName]
		if !ok || p == nil {
			missingProviders++
			continue
		}
		candidates = append(candidates, RouteCandidate{
			ProviderName:   providerName,
			ProviderModel:  providerModel,
			RequestedModel: alias,
			Synthetic:      true,
			Provider:       p,
		})
	}
	if len(candidates) == 0 {
		code := ErrorUnknownModel
		message := fmt.Sprintf("no available provider for synthetic model %q", alias)
		if unsupportedModalities > 0 {
			code = ErrorUnsupportedModality
			message = fmt.Sprintf("synthetic model %q does not support requested modalities", alias)
		} else if missingProviders > 0 {
			code = ErrorNoAvailableModel
		}
		return nil, &Error{Code: code, Message: message, Param: "model"}
	}
	return candidates, nil
}

func (r *StaticRouter) resolveProviderModel(providerName, providerModel, requestedModel string, requiredModalities []string) ([]RouteCandidate, error) {
	providerName = strings.TrimSpace(providerName)
	providerModel = strings.TrimSpace(providerModel)
	if providerName == "" || providerModel == "" {
		return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("unknown model %q", requestedModel), Param: "model"}
	}
	providerCfg, ok := r.cfg.Providers[providerName]
	if !ok {
		return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("unknown provider %q for model %q", providerName, requestedModel), Param: "model"}
	}
	modelCfg, ok := providerCfg.Models[providerModel]
	if !ok {
		return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("unknown model %q for provider %q", providerModel, providerName), Param: "model"}
	}
	if !supportsModalities(modelCfg.Modalities, requiredModalities) {
		return nil, &Error{Code: ErrorUnsupportedModality, Message: fmt.Sprintf("model %q for provider %q does not support requested modalities", providerModel, providerName), Param: "model"}
	}
	return r.candidate(providerName, providerModel, requestedModel)
}

func (r *StaticRouter) resolveDirectModel(model string, requiredModalities []string) ([]RouteCandidate, error) {
	providerNames := sortedProviderNames(r.cfg.Providers)
	foundModel := false
	unsupportedModalities := 0
	for _, providerName := range providerNames {
		providerCfg := r.cfg.Providers[providerName]
		modelCfg, ok := providerCfg.Models[model]
		if !ok {
			continue
		}
		foundModel = true
		if !supportsModalities(modelCfg.Modalities, requiredModalities) {
			unsupportedModalities++
			continue
		}
		return r.candidate(providerName, model, model)
	}
	if foundModel && unsupportedModalities > 0 {
		return nil, &Error{Code: ErrorUnsupportedModality, Message: fmt.Sprintf("model %q does not support requested modalities", model), Param: "model"}
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

func supportsModalities(configured []string, required []string) bool {
	configuredSet := map[string]bool{}
	for _, modality := range configured {
		configuredSet[modality] = true
	}
	// Missing modality metadata is treated as text-only: it can serve legacy
	// text requests but must not receive image requests without explicit image support.
	if len(configuredSet) == 0 {
		configuredSet[ir.ModalityText] = true
	}
	for _, modality := range required {
		if !configuredSet[modality] {
			return false
		}
	}
	return true
}

func sortedProviderNames(providers map[string]config.ProviderConfig) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedModelNames(models map[string]config.ModelConfig) []string {
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedSyntheticNames(models map[string]config.SyntheticModelConfig) []string {
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
