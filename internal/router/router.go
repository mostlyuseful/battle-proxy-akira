// Package router resolves requested model names to provider route candidates.
package router

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
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
	mu           sync.RWMutex
	cfg          config.Config
	providers    map[string]provider.Provider
	availability *AvailabilityTracker
	logger       *slog.Logger
}

// NewStatic creates a deterministic router for configured direct and synthetic models.
func NewStatic(cfg config.Config, providers map[string]provider.Provider) *StaticRouter {
	return NewStaticWithLogger(cfg, providers, nil)
}

// NewStaticWithLogger creates a deterministic router with optional verbose diagnostics.
func NewStaticWithLogger(cfg config.Config, providers map[string]provider.Provider, logger *slog.Logger) *StaticRouter {
	if providers == nil {
		providers = map[string]provider.Provider{}
	}
	if logger != nil {
		logger.Info("router configured", "providers", len(providers), "synthetic_models", len(cfg.SyntheticModels))
	}
	return &StaticRouter{cfg: cfg, providers: providers, availability: NewAvailabilityTracker(), logger: logger}
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
	if r.logger != nil {
		r.logger.Info("resolving model", "requested_model", model, "modalities", requiredModalities)
	}
	if providerName, providerModel, ok := strings.Cut(model, ":"); ok {
		return r.resolveProviderModel(ctx, providerName, providerModel, model, requiredModalities)
	}
	r.mu.RLock()
	synthetic, ok := r.cfg.SyntheticModels[model]
	r.mu.RUnlock()
	if ok {
		return r.resolveSyntheticModel(model, synthetic, requiredModalities)
	}
	return r.resolveDirectModel(ctx, model, requiredModalities)
}

// Models returns configured direct models plus exposed synthetic aliases.
func (r *StaticRouter) Models(ctx context.Context) ([]ir.Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	cfg := r.cfg
	r.mu.RUnlock()

	modelsByID := map[string]ir.Model{}
	for _, providerName := range sortedProviderNames(cfg.Providers) {
		providerCfg := cfg.Providers[providerName]
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

	for _, alias := range sortedSyntheticNames(cfg.SyntheticModels) {
		synthetic := cfg.SyntheticModels[alias]
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
	if r.logger != nil {
		r.logger.Warn("provider candidate marked failed", "provider", candidate.ProviderName, "model", candidate.ProviderModel, "requested_model", candidate.RequestedModel, "error", err)
	}
}

// MarkSuccess records provider/model availability state for the route candidate.
func (r *StaticRouter) MarkSuccess(candidate RouteCandidate) {
	if r.availability != nil {
		r.availability.MarkSuccess(candidate)
	}
	if r.logger != nil {
		r.logger.Info("provider candidate marked successful", "provider", candidate.ProviderName, "model", candidate.ProviderModel, "requested_model", candidate.RequestedModel)
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

// RestoreAvailability replaces tracked availability state with the given snapshots.
// Used during config reload to reconcile state for unchanged provider/model pairs.
func (r *StaticRouter) RestoreAvailability(states []AvailabilityState) {
	if r.availability == nil {
		r.availability = NewAvailabilityTracker()
	}
	r.availability.Restore(states)
	if r.logger != nil {
		r.logger.Info("router availability restored", "states", len(states))
	}
}

func (r *StaticRouter) resolveSyntheticModel(alias string, synthetic config.SyntheticModelConfig, requiredModalities []string) ([]RouteCandidate, error) {
	if synthetic.Strategy != config.SyntheticStrategyFirstAvailable {
		if r.logger != nil {
			r.logger.Warn("synthetic model rejected", "model", alias, "strategy", synthetic.Strategy, "reason", "unsupported_strategy")
		}
		return nil, &Error{Code: ErrorNoAvailableModel, Message: fmt.Sprintf("synthetic model %q uses unsupported strategy %q", alias, synthetic.Strategy), Param: "model"}
	}

	candidates := make([]RouteCandidate, 0, len(synthetic.Candidates))
	missingProviders := 0
	unavailableCandidates := 0
	unsupportedModalities := 0
	for _, candidate := range synthetic.Candidates {
		providerName, providerModel, ok := strings.Cut(candidate, ":")
		if !ok || providerName == "" || providerModel == "" {
			if r.logger != nil {
				r.logger.Warn("synthetic candidate rejected", "synthetic_model", alias, "candidate", candidate, "reason", "invalid_reference")
			}
			return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("synthetic model %q contains invalid candidate %q", alias, candidate), Param: "model"}
		}
		r.mu.RLock()
		providerCfg, ok := r.cfg.Providers[providerName]
		r.mu.RUnlock()
		if !ok {
			if r.logger != nil {
				r.logger.Warn("synthetic candidate rejected", "synthetic_model", alias, "candidate", candidate, "reason", "unknown_provider", "provider", providerName)
			}
			return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("synthetic model %q references unknown provider %q", alias, providerName), Param: "model"}
		}
		modelCfg, ok := providerCfg.Models[providerModel]
		if !ok {
			if r.logger != nil {
				r.logger.Warn("synthetic candidate rejected", "synthetic_model", alias, "candidate", candidate, "reason", "unknown_model", "provider", providerName, "model", providerModel)
			}
			return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("synthetic model %q references unknown model %q for provider %q", alias, providerModel, providerName), Param: "model"}
		}
		if !supportsModalities(modelCfg.Modalities, requiredModalities) {
			unsupportedModalities++
			if r.logger != nil {
				r.logger.Info("synthetic candidate skipped", "synthetic_model", alias, "candidate", candidate, "reason", "unsupported_modality", "required_modalities", requiredModalities, "model_modalities", modelCfg.Modalities)
			}
			continue
		}
		p, ok := r.providers[providerName]
		if !ok || p == nil {
			missingProviders++
			if r.logger != nil {
				r.logger.Warn("synthetic candidate skipped", "synthetic_model", alias, "candidate", candidate, "reason", "missing_provider", "provider", providerName)
			}
			continue
		}
		if r.availability != nil && !r.availability.IsAvailable(providerName, providerModel, time.Now()) {
			unavailableCandidates++
			if r.logger != nil {
				r.logger.Info("synthetic candidate skipped", "synthetic_model", alias, "candidate", candidate, "reason", "unavailable", "provider", providerName, "model", providerModel)
			}
			continue
		}
		candidates = append(candidates, RouteCandidate{
			ProviderName:   providerName,
			ProviderModel:  providerModel,
			RequestedModel: alias,
			Synthetic:      true,
			Provider:       p,
		})
		if r.logger != nil {
			r.logger.Info("synthetic candidate accepted", "synthetic_model", alias, "candidate", candidate, "provider", providerName, "model", providerModel)
		}
	}
	if len(candidates) == 0 {
		code := ErrorUnknownModel
		message := fmt.Sprintf("no available provider for synthetic model %q", alias)
		if unsupportedModalities > 0 {
			code = ErrorUnsupportedModality
			message = fmt.Sprintf("synthetic model %q does not support requested modalities", alias)
		} else if missingProviders > 0 || unavailableCandidates > 0 {
			code = ErrorNoAvailableModel
		}
		if r.logger != nil {
			r.logger.Warn("synthetic model has no available candidates", "model", alias, "reason", message, "unsupported_modalities", unsupportedModalities, "missing_providers", missingProviders, "unavailable_candidates", unavailableCandidates)
		}
		return nil, &Error{Code: code, Message: message, Param: "model"}
	}
	if r.logger != nil {
		r.logger.Info("synthetic model resolved", "model", alias, "candidates", len(candidates))
	}
	return candidates, nil
}

func (r *StaticRouter) resolveProviderModel(ctx context.Context, providerName, providerModel, requestedModel string, requiredModalities []string) ([]RouteCandidate, error) {
	providerName = strings.TrimSpace(providerName)
	providerModel = strings.TrimSpace(providerModel)
	if providerName == "" || providerModel == "" {
		if r.logger != nil {
			r.logger.Info("provider-qualified model rejected", "requested_model", requestedModel, "reason", "empty_provider_or_model")
		}
		return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("unknown model %q", requestedModel), Param: "model"}
	}

	r.mu.RLock()
	providerCfg, ok := r.cfg.Providers[providerName]
	r.mu.RUnlock()
	if !ok {
		if r.logger != nil {
			r.logger.Warn("provider-qualified model rejected", "requested_model", requestedModel, "provider", providerName, "reason", "unknown_provider")
		}
		return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("unknown provider %q for model %q", providerName, requestedModel), Param: "model"}
	}
	if _, ok := providerCfg.Models[providerModel]; !ok {
		if len(providerCfg.Models) == 0 {
			providerCfg, _ = r.refreshProviderModels(ctx, providerName)
		}
		if _, ok := providerCfg.Models[providerModel]; !ok {
			if len(providerCfg.Models) == 0 {
				if r.logger != nil {
					r.logger.Warn("provider-qualified model unavailable", "requested_model", requestedModel, "provider", providerName, "model", providerModel, "reason", "provider_offline_or_undiscovered")
				}
				return nil, &Error{Code: ErrorNoAvailableModel, Message: fmt.Sprintf("provider %q is unavailable", providerName), Param: "model"}
			}
			if r.logger != nil {
				r.logger.Warn("provider-qualified model rejected", "requested_model", requestedModel, "provider", providerName, "model", providerModel, "reason", "unknown_model")
			}
			return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("unknown model %q for provider %q", providerModel, providerName), Param: "model"}
		}
	}
	if r.availability != nil && !r.availability.IsAvailable(providerName, providerModel, time.Now()) {
		if r.logger != nil {
			r.logger.Info("provider-qualified model rejected", "requested_model", requestedModel, "provider", providerName, "model", providerModel, "reason", "unavailable")
		}
		return nil, &Error{Code: ErrorNoAvailableModel, Message: fmt.Sprintf("model %q for provider %q is temporarily unavailable", providerModel, providerName), Param: "model"}
	}
	return r.candidate(providerName, providerModel, requestedModel)
}

func (r *StaticRouter) resolveDirectModel(ctx context.Context, model string, requiredModalities []string) ([]RouteCandidate, error) {
	r.mu.RLock()
	providerNames := sortedProviderNames(r.cfg.Providers)
	r.mu.RUnlock()
	foundModel := false
	unavailableCandidates := 0
	discoveryFailures := 0
	for _, providerName := range providerNames {
		r.mu.RLock()
		providerCfg := r.cfg.Providers[providerName]
		r.mu.RUnlock()
		if _, ok := providerCfg.Models[model]; !ok {
			if len(providerCfg.Models) == 0 {
				refreshed, err := r.refreshProviderModels(ctx, providerName)
				if err != nil {
					discoveryFailures++
				}
				providerCfg = refreshed
			}
			if _, ok := providerCfg.Models[model]; !ok {
				continue
			}
		}
		foundModel = true
		if r.availability != nil && !r.availability.IsAvailable(providerName, model, time.Now()) {
			unavailableCandidates++
			if r.logger != nil {
				r.logger.Info("direct model candidate skipped", "model", model, "provider", providerName, "reason", "unavailable")
			}
			continue
		}
		if r.logger != nil {
			r.logger.Info("direct model resolved", "model", model, "provider", providerName, "required_modalities", requiredModalities)
		}
		return r.candidate(providerName, model, model)
	}
	if foundModel && unavailableCandidates > 0 {
		if r.logger != nil {
			r.logger.Info("direct model rejected", "model", model, "reason", "unavailable", "unavailable_candidates", unavailableCandidates)
		}
		return nil, &Error{Code: ErrorNoAvailableModel, Message: fmt.Sprintf("model %q is temporarily unavailable", model), Param: "model"}
	}
	if discoveryFailures > 0 {
		if r.logger != nil {
			r.logger.Info("model unavailable after lazy discovery attempts", "model", model, "discovery_failures", discoveryFailures)
		}
		return nil, &Error{Code: ErrorNoAvailableModel, Message: fmt.Sprintf("model %q is temporarily unavailable", model), Param: "model"}
	}
	if r.logger != nil {
		r.logger.Info("model not found", "model", model)
	}
	return nil, &Error{Code: ErrorUnknownModel, Message: fmt.Sprintf("unknown model %q", model), Param: "model"}
}

func (r *StaticRouter) candidate(providerName, providerModel, requestedModel string) ([]RouteCandidate, error) {
	p, ok := r.providers[providerName]
	if !ok || p == nil {
		if r.logger != nil {
			r.logger.Warn("candidate rejected", "provider", providerName, "model", providerModel, "requested_model", requestedModel, "reason", "missing_provider")
		}
		return nil, &Error{Code: ErrorNoAvailableModel, Message: fmt.Sprintf("no available provider for model %q", requestedModel), Param: "model"}
	}
	candidates := []RouteCandidate{
		{
			ProviderName:   providerName,
			ProviderModel:  providerModel,
			RequestedModel: requestedModel,
			Provider:       p,
		},
	}
	if r.logger != nil {
		r.logger.Info("route candidate selected", "provider", providerName, "model", providerModel, "requested_model", requestedModel)
	}
	return candidates, nil
}

func (r *StaticRouter) refreshProviderModels(ctx context.Context, providerName string) (config.ProviderConfig, error) {
	r.mu.RLock()
	providerCfg, ok := r.cfg.Providers[providerName]
	p := r.providers[providerName]
	r.mu.RUnlock()
	if !ok {
		return config.ProviderConfig{}, fmt.Errorf("unknown provider")
	}
	if len(providerCfg.Models) > 0 {
		return providerCfg, nil
	}
	if p == nil {
		return providerCfg, fmt.Errorf("provider is not configured")
	}
	models, err := p.Models(ctx)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("lazy provider model discovery failed", "provider", providerName, "error", err)
		}
		return providerCfg, err
	}
	providerCfg.Models = make(map[string]config.ModelConfig, len(models))
	for _, model := range models {
		providerCfg.Models[model.ID] = config.ModelConfig{Modalities: append([]string(nil), model.Modalities...)}
	}

	r.mu.Lock()
	current := r.cfg.Providers[providerName]
	if len(current.Models) == 0 {
		current.Models = providerCfg.Models
		r.cfg.Providers[providerName] = current
		providerCfg = current
	} else {
		providerCfg = current
	}
	r.mu.Unlock()
	if r.logger != nil {
		r.logger.Info("lazy provider model discovery succeeded", "provider", providerName, "models", len(providerCfg.Models))
	}
	return providerCfg, nil
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
