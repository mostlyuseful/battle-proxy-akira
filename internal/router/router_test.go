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
