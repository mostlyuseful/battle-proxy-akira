package router

import (
	"context"
	"errors"
	"testing"

	"battle-proxy-akira/internal/config"
	"battle-proxy-akira/internal/ir"
	"battle-proxy-akira/internal/provider"
)

type fakeProvider struct{ name string }

func (p fakeProvider) Name() string { return p.name }
func (p fakeProvider) Complete(context.Context, ir.Request) (*ir.Response, error) {
	return &ir.Response{}, nil
}
func (p fakeProvider) Stream(context.Context, ir.Request) (<-chan ir.Event, error) {
	return nil, nil
}
func (p fakeProvider) Models(context.Context) ([]ir.Model, error) { return nil, nil }
func (p fakeProvider) Health(context.Context) error               { return nil }

func TestResolveDirectModel(t *testing.T) {
	t.Parallel()

	r := NewStatic(testConfig(), map[string]provider.Provider{
		"openai_api": fakeProvider{name: "openai_api"},
	})

	candidates, err := r.Resolve(context.Background(), ir.Request{Model: "gpt-5.2"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates length = %d, want 1", len(candidates))
	}
	candidate := candidates[0]
	if candidate.ProviderName != "openai_api" {
		t.Fatalf("ProviderName = %q, want openai_api", candidate.ProviderName)
	}
	if candidate.ProviderModel != "gpt-5.2" {
		t.Fatalf("ProviderModel = %q, want gpt-5.2", candidate.ProviderModel)
	}
	if candidate.RequestedModel != "gpt-5.2" {
		t.Fatalf("RequestedModel = %q, want gpt-5.2", candidate.RequestedModel)
	}
	if candidate.Provider == nil {
		t.Fatal("Provider is nil")
	}
}

func TestResolveProviderModelNotation(t *testing.T) {
	t.Parallel()

	r := NewStatic(testConfig(), map[string]provider.Provider{
		"codex_sub": fakeProvider{name: "codex_sub"},
	})

	candidates, err := r.Resolve(context.Background(), ir.Request{Model: "codex_sub:gpt-5.1-codex-max"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	candidate := candidates[0]
	if candidate.ProviderName != "codex_sub" {
		t.Fatalf("ProviderName = %q, want codex_sub", candidate.ProviderName)
	}
	if candidate.ProviderModel != "gpt-5.1-codex-max" {
		t.Fatalf("ProviderModel = %q, want gpt-5.1-codex-max", candidate.ProviderModel)
	}
	if candidate.RequestedModel != "codex_sub:gpt-5.1-codex-max" {
		t.Fatalf("RequestedModel = %q", candidate.RequestedModel)
	}
}

func TestResolveDirectModelIsDeterministic(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Providers["a_provider"] = config.ProviderConfig{
		Models: map[string]config.ModelConfig{"shared": {Modalities: []string{ir.ModalityText}}},
	}
	cfg.Providers["z_provider"] = config.ProviderConfig{
		Models: map[string]config.ModelConfig{"shared": {Modalities: []string{ir.ModalityText}}},
	}
	r := NewStatic(cfg, map[string]provider.Provider{
		"a_provider": fakeProvider{name: "a_provider"},
		"z_provider": fakeProvider{name: "z_provider"},
	})

	candidates, err := r.Resolve(context.Background(), ir.Request{Model: "shared"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if candidates[0].ProviderName != "a_provider" {
		t.Fatalf("ProviderName = %q, want lexicographically first a_provider", candidates[0].ProviderName)
	}
}

func TestResolveTextOnlyRequestCanUseTextModel(t *testing.T) {
	t.Parallel()

	r := NewStatic(testConfig(), map[string]provider.Provider{
		"codex_sub": fakeProvider{name: "codex_sub"},
	})

	candidates, err := r.Resolve(context.Background(), ir.Request{
		Model:    "codex_sub:gpt-5.1-codex-max",
		Messages: []ir.Message{{Role: ir.RoleUser, Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "hello"}}}},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ProviderName != "codex_sub" {
		t.Fatalf("candidates = %#v, want codex_sub text model", candidates)
	}
}

func TestResolveImageRequestUsesImageCapableDirectModel(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Providers["a_text"] = config.ProviderConfig{Models: map[string]config.ModelConfig{"shared": {Modalities: []string{ir.ModalityText}}}}
	cfg.Providers["z_vision"] = config.ProviderConfig{Models: map[string]config.ModelConfig{"shared": {Modalities: []string{ir.ModalityText, ir.ModalityImage}}}}
	r := NewStatic(cfg, map[string]provider.Provider{
		"a_text":   fakeProvider{name: "a_text"},
		"z_vision": fakeProvider{name: "z_vision"},
	})

	candidates, err := r.Resolve(context.Background(), imageRequest("shared"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ProviderName != "z_vision" {
		t.Fatalf("candidates = %#v, want z_vision image-capable model", candidates)
	}
}

func TestResolveImageRequestFiltersSyntheticCandidates(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithSynthetic()
	r := NewStatic(cfg, map[string]provider.Provider{
		"codex_sub":  fakeProvider{name: "codex_sub"},
		"openai_api": fakeProvider{name: "openai_api"},
	})

	candidates, err := r.Resolve(context.Background(), imageRequest("coding"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ProviderName != "openai_api" || candidates[0].ProviderModel != "gpt-5.2" {
		t.Fatalf("candidates = %#v, want only openai_api gpt-5.2", candidates)
	}
}

func TestResolveImageRequestUnsupportedModality(t *testing.T) {
	t.Parallel()

	r := NewStatic(testConfig(), map[string]provider.Provider{
		"codex_sub": fakeProvider{name: "codex_sub"},
	})

	_, err := r.Resolve(context.Background(), imageRequest("codex_sub:gpt-5.1-codex-max"))
	assertRouterError(t, err, ErrorUnsupportedModality)
}

func TestResolveMissingModalityMetadataIsTextOnly(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Providers["legacy"] = config.ProviderConfig{Models: map[string]config.ModelConfig{"legacy-model": {}}}
	r := NewStatic(cfg, map[string]provider.Provider{"legacy": fakeProvider{name: "legacy"}})

	if _, err := r.Resolve(context.Background(), ir.Request{Model: "legacy-model"}); err != nil {
		t.Fatalf("Resolve text request with missing modalities: %v", err)
	}
	_, err := r.Resolve(context.Background(), imageRequest("legacy-model"))
	assertRouterError(t, err, ErrorUnsupportedModality)
}

func TestResolveUnknownModel(t *testing.T) {
	t.Parallel()

	r := NewStatic(testConfig(), map[string]provider.Provider{
		"openai_api": fakeProvider{name: "openai_api"},
	})

	_, err := r.Resolve(context.Background(), ir.Request{Model: "missing"})
	assertRouterError(t, err, ErrorUnknownModel)
}

func TestResolveConfiguredModelWithoutProviderInstance(t *testing.T) {
	t.Parallel()

	r := NewStatic(testConfig(), nil)

	_, err := r.Resolve(context.Background(), ir.Request{Model: "gpt-5.2"})
	assertRouterError(t, err, ErrorNoAvailableModel)
}

func TestResolveProviderModelUnknownProvider(t *testing.T) {
	t.Parallel()

	r := NewStatic(testConfig(), nil)

	_, err := r.Resolve(context.Background(), ir.Request{Model: "missing:gpt-5.2"})
	assertRouterError(t, err, ErrorUnknownModel)
}

func TestResolveHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	r := NewStatic(testConfig(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Resolve(ctx, ir.Request{Model: "gpt-5.2"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve error = %v, want context.Canceled", err)
	}
}

func TestResolveSyntheticModelAliasInConfiguredOrder(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithSynthetic()
	r := NewStatic(cfg, map[string]provider.Provider{
		"codex_sub":  fakeProvider{name: "codex_sub"},
		"openai_api": fakeProvider{name: "openai_api"},
	})

	candidates, err := r.Resolve(context.Background(), ir.Request{Model: "coding"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates length = %d, want 2", len(candidates))
	}
	if candidates[0].ProviderName != "codex_sub" || candidates[0].ProviderModel != "gpt-5.1-codex-max" {
		t.Fatalf("first candidate = %#v", candidates[0])
	}
	if candidates[1].ProviderName != "openai_api" || candidates[1].ProviderModel != "gpt-5.2" {
		t.Fatalf("second candidate = %#v", candidates[1])
	}
	for _, candidate := range candidates {
		if candidate.RequestedModel != "coding" {
			t.Fatalf("RequestedModel = %q, want coding", candidate.RequestedModel)
		}
		if !candidate.Synthetic {
			t.Fatalf("Synthetic = false for candidate %#v", candidate)
		}
	}
}

func TestResolveSyntheticModelSkipsMissingProviderInstances(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithSynthetic()
	r := NewStatic(cfg, map[string]provider.Provider{
		"openai_api": fakeProvider{name: "openai_api"},
	})

	candidates, err := r.Resolve(context.Background(), ir.Request{Model: "coding"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ProviderName != "openai_api" {
		t.Fatalf("candidates = %#v, want only openai_api", candidates)
	}
}

func TestResolveSyntheticModelNoAvailableProviders(t *testing.T) {
	t.Parallel()

	r := NewStatic(testConfigWithSynthetic(), nil)

	_, err := r.Resolve(context.Background(), ir.Request{Model: "coding"})
	assertRouterError(t, err, ErrorNoAvailableModel)
}

func TestModelsIncludesExposedSyntheticAliases(t *testing.T) {
	t.Parallel()

	r := NewStatic(testConfigWithSynthetic(), nil)

	models, err := r.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	byID := map[string]ir.Model{}
	for _, model := range models {
		byID[model.ID] = model
	}
	if !byID["coding"].Synthetic {
		t.Fatalf("coding model = %#v, want synthetic", byID["coding"])
	}
	if _, ok := byID["hidden_alias"]; ok {
		t.Fatalf("hidden_alias should not be exposed: %#v", models)
	}
	if byID["gpt-5.2"].Synthetic {
		t.Fatalf("direct model gpt-5.2 should not be synthetic")
	}
}

func TestRouteCandidateProviderRequestAndResponseRewrite(t *testing.T) {
	t.Parallel()

	candidate := RouteCandidate{
		ProviderName:   "codex_sub",
		ProviderModel:  "gpt-5.1-codex-max",
		RequestedModel: "coding",
		Synthetic:      true,
	}
	providerReq := candidate.ProviderRequest(ir.Request{Model: "coding"})
	if providerReq.Model != "gpt-5.1-codex-max" {
		t.Fatalf("provider request model = %q, want provider model", providerReq.Model)
	}
	rewritten := candidate.RewriteResponse(ir.Response{Model: "gpt-5.1-codex-max"})
	if rewritten.Model != "coding" {
		t.Fatalf("rewritten response model = %q, want coding", rewritten.Model)
	}
}

func TestResolveUnknownAliasFallsThroughToUnknownModel(t *testing.T) {
	t.Parallel()

	r := NewStatic(testConfigWithSynthetic(), nil)

	_, err := r.Resolve(context.Background(), ir.Request{Model: "unknown_alias"})
	assertRouterError(t, err, ErrorUnknownModel)
}

func imageRequest(model string) ir.Request {
	return ir.Request{
		Model: model,
		Messages: []ir.Message{{
			Role: ir.RoleUser,
			Content: []ir.ContentPart{
				{Type: ir.ContentTypeText, Text: "describe"},
				{Type: ir.ContentTypeImageURL, ImageURL: "data:image/png;base64,abc"},
			},
		}},
	}
}

func assertRouterError(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want router code %s", wantCode)
	}
	var routerErr *Error
	if !errors.As(err, &routerErr) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if routerErr.Code != wantCode {
		t.Fatalf("router error code = %q, want %q", routerErr.Code, wantCode)
	}
	if routerErr.Param != "model" {
		t.Fatalf("router error param = %q, want model", routerErr.Param)
	}
}

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"openai_api": {
			Models: map[string]config.ModelConfig{
				"gpt-5.2": {Modalities: []string{ir.ModalityText, ir.ModalityImage}},
			},
		},
		"codex_sub": {
			Models: map[string]config.ModelConfig{
				"gpt-5.1-codex-max": {Modalities: []string{ir.ModalityText}},
			},
		},
	}
	return cfg
}

func testConfigWithSynthetic() config.Config {
	cfg := testConfig()
	cfg.SyntheticModels = map[string]config.SyntheticModelConfig{
		"coding": {
			Strategy: config.SyntheticStrategyFirstAvailable,
			Expose:   true,
			Candidates: []string{
				"codex_sub:gpt-5.1-codex-max",
				"openai_api:gpt-5.2",
			},
		},
		"hidden_alias": {
			Strategy:   config.SyntheticStrategyFirstAvailable,
			Expose:     false,
			Candidates: []string{"openai_api:gpt-5.2"},
		},
	}
	return cfg
}
