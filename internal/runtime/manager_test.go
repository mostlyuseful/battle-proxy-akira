package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"battle-proxy-akira/internal/config"
	"battle-proxy-akira/internal/ir"
	"battle-proxy-akira/internal/provider"
	"battle-proxy-akira/internal/router"
)

// validConfig returns a config that passes config.Validate and builds providers
// with bearer_env auth (env read lazily at request time, so building never needs it set).
func validConfig(extraProvider string) *config.Config {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"openai_api": {
			Type:    config.ProviderTypeOpenAICompatible,
			BaseURL: "https://api.openai.com/v1",
			Auth:    config.AuthConfig{Type: config.AuthTypeBearerEnv, Env: "OPENAI_API_KEY"},
			Models: map[string]config.ModelConfig{
				"gpt-5.2": {Modalities: []string{ir.ModalityText, ir.ModalityImage}},
			},
		},
	}
	if extraProvider != "" {
		cfg.Providers["codex_sub"] = config.ProviderConfig{
			Type:    config.ProviderTypeOpenAICompatible,
			BaseURL: "https://api.openai.com/v1",
			Auth:    config.AuthConfig{Type: config.AuthTypeBearerEnv, Env: "CODEX_API_KEY"},
			Models: map[string]config.ModelConfig{
				"gpt-5.1-codex-max": {Modalities: []string{ir.ModalityText}},
			},
		}
	}
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	return &cfg
}

// validConfigWithoutOpenAI removes the openai_api provider, used to test that
// availability state for removed pairs is dropped on reload.
func validDynamicConfig(baseURL string) *config.Config {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"openai_api": {
			Type:    config.ProviderTypeOpenAICompatible,
			BaseURL: baseURL,
			Auth:    config.AuthConfig{Type: config.AuthTypeBearerValue, Value: "sk-inline-secret"},
		},
	}
	cfg.SyntheticModels = map[string]config.SyntheticModelConfig{
		"coding": {
			Strategy:   config.SyntheticStrategyFirstAvailable,
			Expose:     true,
			Candidates: []string{"openai_api:gpt-dynamic"},
		},
	}
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	return &cfg
}

func validConfigWithoutOpenAI() *config.Config {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"codex_sub": {
			Type:    config.ProviderTypeOpenAICompatible,
			BaseURL: "https://api.openai.com/v1",
			Auth:    config.AuthConfig{Type: config.AuthTypeBearerEnv, Env: "CODEX_API_KEY"},
			Models: map[string]config.ModelConfig{
				"gpt-5.1-codex-max": {Modalities: []string{ir.ModalityText}},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	return &cfg
}

func TestManagerBuildDiscoversProviderModelsForRouting(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-inline-secret" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-dynamic"}]}`))
	}))
	defer upstream.Close()

	m, err := NewManager(func() (*config.Config, error) {
		return validDynamicConfig(upstream.URL + "/v1"), nil
	}, upstream.Client())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	models, err := m.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if !containsModelID(models, "gpt-dynamic") || !containsModelID(models, "coding") {
		t.Fatalf("models = %v, want discovered direct and synthetic models", modelIDs(models))
	}
	candidates, err := m.Resolve(context.Background(), ir.Request{Model: "coding"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ProviderModel != "gpt-dynamic" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestManagerReloadSuccessSwapsConfigAndProviders(t *testing.T) {
	t.Parallel()

	var calls int32
	loader := func() (*config.Config, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return validConfig(""), nil
		default:
			return validConfig("codex_sub"), nil
		}
	}

	m, err := NewManager(loader, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	models, err := m.Models(context.Background())
	if err != nil {
		t.Fatalf("initial Models: %v", err)
	}
	if containsModelID(models, "gpt-5.1-codex-max") {
		t.Fatalf("initial models should not yet include codex model, got %v", modelIDs(models))
	}

	if err := m.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	models, err = m.Models(context.Background())
	if err != nil {
		t.Fatalf("reloaded Models: %v", err)
	}
	if !containsModelID(models, "gpt-5.1-codex-max") {
		t.Fatalf("reloaded models should include codex model, got %v", modelIDs(models))
	}
	if !containsModelID(models, "gpt-5.2") {
		t.Fatalf("reloaded models should still include gpt-5.2, got %v", modelIDs(models))
	}

	// New provider instance is built for the reloaded snapshot.
	snap := m.Current()
	if snap.Providers["codex_sub"] == nil {
		t.Fatal("reloaded snapshot missing codex_sub provider")
	}
}

func TestManagerReloadFailedValidationRetainsOldConfig(t *testing.T) {
	t.Parallel()

	initial := validConfig("")
	loaderErr := errors.New("boom: config file missing")

	var calls int32
	loader := func() (*config.Config, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return initial, nil
		default:
			return nil, loaderErr
		}
	}

	m, err := NewManager(loader, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	before := m.Current()
	if err := m.Reload(); !errors.Is(err, loaderErr) {
		t.Fatalf("Reload error = %v, want %v", err, loaderErr)
	}

	after := m.Current()
	if after != before {
		t.Fatal("snapshot changed after failed reload; old config must be retained")
	}
	models, err := m.Models(context.Background())
	if err != nil {
		t.Fatalf("Models after failed reload: %v", err)
	}
	if !containsModelID(models, "gpt-5.2") {
		t.Fatalf("old config models must still be served after failed reload, got %v", modelIDs(models))
	}
}

func TestManagerReloadFailedValidationRetainsOldConfigOnInvalidConfig(t *testing.T) {
	t.Parallel()

	// Second load returns a config that fails validation (empty provider type).
	bad := config.Default()
	bad.Providers = map[string]config.ProviderConfig{
		"broken": {BaseURL: "https://api.openai.com/v1", Models: map[string]config.ModelConfig{"m": {Modalities: []string{ir.ModalityText}}}},
	}

	var calls int32
	loader := func() (*config.Config, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return validConfig(""), nil
		default:
			return &bad, nil
		}
	}

	m, err := NewManager(loader, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	before := m.Current()

	if err := m.Reload(); err == nil {
		t.Fatal("Reload with invalid config must return an error")
	}
	if m.Current() != before {
		t.Fatal("snapshot changed after invalid reload; old config must be retained")
	}
}

func TestManagerReloadDoesNotInterruptInFlightCandidates(t *testing.T) {
	t.Parallel()

	var calls int32
	loader := func() (*config.Config, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return validConfig(""), nil
		default:
			return validConfig("codex_sub"), nil
		}
	}

	m, err := NewManager(loader, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Resolve a candidate against the initial snapshot, then capture its provider.
	candidates, err := m.Resolve(context.Background(), ir.Request{Model: "gpt-5.2"})
	if err != nil {
		t.Fatalf("initial Resolve: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("initial candidates = %d, want 1", len(candidates))
	}
	inFlightProvider := candidates[0].Provider
	if inFlightProvider == nil {
		t.Fatal("in-flight candidate provider is nil")
	}
	inFlightProviderName := inFlightProvider.Name()

	// Reload swaps the active snapshot atomically.
	if err := m.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	// The in-flight candidate's provider instance remains valid and usable.
	if inFlightProvider.Name() != inFlightProviderName {
		t.Fatalf("in-flight provider name changed after reload: %q", inFlightProvider.Name())
	}
	inFlightModels, err := inFlightProvider.Models(context.Background())
	if err != nil {
		t.Fatalf("in-flight provider Models after reload: %v", err)
	}
	if !containsModelID(inFlightModels, "gpt-5.2") {
		t.Fatalf("in-flight provider should still serve gpt-5.2, got %v", modelIDs(inFlightModels))
	}

	// New requests resolve against the reloaded snapshot and see the new provider.
	reloadedCandidates, err := m.Resolve(context.Background(), ir.Request{Model: "gpt-5.1-codex-max"})
	if err != nil {
		t.Fatalf("reloaded Resolve: %v", err)
	}
	if reloadedCandidates[0].Provider == inFlightProvider {
		t.Fatal("reloaded candidate should use a fresh provider instance for codex_sub")
	}
}

func TestManagerReloadPreservesAvailabilityForUnchangedPairs(t *testing.T) {
	t.Parallel()

	var calls int32
	loader := func() (*config.Config, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return validConfig(""), nil
		default:
			return validConfig(""), nil
		}
	}

	m, err := NewManager(loader, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	candidates, err := m.Resolve(context.Background(), ir.Request{Model: "gpt-5.2"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// Mark a rate-limit failure so an exhaustion window is recorded.
	rateLimitErr := &provider.Error{Code: provider.ErrorProviderRateLimited, Retryable: true, Provider: "openai_api"}
	m.MarkFailure(candidates[0], rateLimitErr)

	state, ok := m.Current().Router.Availability("openai_api", "gpt-5.2")
	if !ok {
		t.Fatal("expected availability state before reload")
	}
	if state.ExhaustedUntil == nil {
		t.Fatal("expected exhaustion window before reload")
	}
	if m.Current().Router.IsCandidateAvailable(candidates[0], time.Now()) {
		t.Fatal("expected candidate unavailable before reload")
	}

	// Reload keeps openai_api:gpt-5.2, so the exhaustion state must be reconciled.
	if err := m.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	// The exhaustion window for openai_api:gpt-5.2 must be reconciled into the
	// reloaded snapshot. Resolve would filter the candidate as unavailable, so
	// inspect availability state directly.
	preserved, ok := m.Current().Router.Availability("openai_api", "gpt-5.2")
	if !ok {
		t.Fatal("expected availability state preserved across reload for unchanged pair")
	}
	if preserved.ExhaustedUntil == nil {
		t.Fatal("expected exhaustion window preserved across reload for unchanged pair")
	}
	if preserved.LastErrorCode != provider.ErrorProviderRateLimited {
		t.Fatalf("expected last error code preserved, got %q", preserved.LastErrorCode)
	}
	if m.Current().Router.IsCandidateAvailable(router.RouteCandidate{ProviderName: "openai_api", ProviderModel: "gpt-5.2"}, time.Now()) {
		t.Fatal("expected candidate to remain unavailable across reload for unchanged pair")
	}
}

func TestManagerReloadDropsAvailabilityForRemovedPairs(t *testing.T) {
	t.Parallel()

	var calls int32
	loader := func() (*config.Config, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return validConfig(""), nil
		default:
			return validConfigWithoutOpenAI(), nil
		}
	}

	m, err := NewManager(loader, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	candidates, err := m.Resolve(context.Background(), ir.Request{Model: "gpt-5.2"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	rateLimitErr := &provider.Error{Code: provider.ErrorProviderRateLimited, Retryable: true, Provider: "openai_api"}
	m.MarkFailure(candidates[0], rateLimitErr)

	if err := m.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	for _, state := range m.Current().Router.AvailabilityStates() {
		if state.Provider == "openai_api" {
			t.Fatalf("availability state for removed openai_api pair must be dropped, got %+v", state)
		}
	}
	// The remaining provider's model still resolves.
	if _, err := m.Resolve(context.Background(), ir.Request{Model: "gpt-5.1-codex-max"}); err != nil {
		t.Fatalf("Resolve codex after reload: %v", err)
	}
}

func TestManagerResolveBeforeInitReturnsNoAvailableModel(t *testing.T) {
	t.Parallel()

	// Build a manager, then clear its snapshot to simulate uninitialized state.
	m, err := NewManager(func() (*config.Config, error) { return validConfig(""), nil }, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	m.current.Store(nil)

	_, err = m.Resolve(context.Background(), ir.Request{Model: "gpt-5.2"})
	var routerErr *router.Error
	if !errors.As(err, &routerErr) || routerErr.Code != router.ErrorNoAvailableModel {
		t.Fatalf("Resolve error = %v, want no_available_model", err)
	}
}

func containsModelID(models []ir.Model, id string) bool {
	for _, model := range models {
		if model.ID == id {
			return true
		}
	}
	return false
}

func modelIDs(models []ir.Model) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}
