// Package runtime owns the live, reloadable proxy runtime: the parsed config,
// the built provider adapters, and the router. A Manager swaps these atomically
// so in-flight requests keep using their resolved snapshot while new requests
// observe the reloaded configuration.
package runtime

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"battle-proxy-akira/internal/auth"
	"battle-proxy-akira/internal/config"
	"battle-proxy-akira/internal/ir"
	"battle-proxy-akira/internal/provider"
	"battle-proxy-akira/internal/router"
)

// Snapshot is an immutable runtime view built from one validated config.
type Snapshot struct {
	Config    *config.Config
	Router    *router.StaticRouter
	Providers map[string]provider.Provider
}

// Manager holds the current runtime snapshot and reloads it on demand.
// Reloads are serialized; reads are lock-free via an atomic pointer.
type Manager struct {
	load       func() (*config.Config, error)
	httpClient *http.Client

	mu      sync.Mutex // serializes Reload
	current atomic.Pointer[Snapshot]
}

// NewManager loads the initial config and builds the first snapshot.
// load must return a freshly validated *config.Config on each call.
// httpClient is used for all upstream provider requests; nil defaults to http.DefaultClient.
func NewManager(load func() (*config.Config, error), httpClient *http.Client) (*Manager, error) {
	if load == nil {
		return nil, fmt.Errorf("runtime: load function is required")
	}
	m := &Manager{load: load, httpClient: httpClient}
	snap, err := m.build()
	if err != nil {
		return nil, err
	}
	m.current.Store(snap)
	return m, nil
}

// Current returns the active runtime snapshot.
func (m *Manager) Current() *Snapshot {
	return m.current.Load()
}

// Reload validates a new config, builds fresh providers and router, reconciles
// availability state for unchanged provider/model pairs, and atomically swaps
// the active snapshot. On any failure the previous snapshot remains active.
func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	snap, err := m.build()
	if err != nil {
		return err
	}

	if old := m.current.Load(); old != nil && old.Router != nil && snap.Router != nil {
		snap.Router.RestoreAvailability(reconcileAvailability(old.Router.AvailabilityStates(), snap.Config))
	}

	m.current.Store(snap)
	return nil
}

func (m *Manager) build() (*Snapshot, error) {
	cfg, err := m.load()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("runtime: loaded config is nil")
	}
	providers, err := buildProviders(*cfg, m.httpClient)
	if err != nil {
		return nil, err
	}
	r := router.NewStatic(*cfg, providers)
	return &Snapshot{Config: cfg, Router: r, Providers: providers}, nil
}

// Resolve delegates to the current snapshot's router.
func (m *Manager) Resolve(ctx context.Context, req ir.Request) ([]router.RouteCandidate, error) {
	snap := m.current.Load()
	if snap == nil || snap.Router == nil {
		return nil, &router.Error{Code: router.ErrorNoAvailableModel, Message: "proxy runtime is not initialized", Param: "model"}
	}
	return snap.Router.Resolve(ctx, req)
}

// MarkFailure delegates to the current snapshot's router.
func (m *Manager) MarkFailure(candidate router.RouteCandidate, err error) {
	if snap := m.current.Load(); snap != nil && snap.Router != nil {
		snap.Router.MarkFailure(candidate, err)
	}
}

// MarkSuccess delegates to the current snapshot's router.
func (m *Manager) MarkSuccess(candidate router.RouteCandidate) {
	if snap := m.current.Load(); snap != nil && snap.Router != nil {
		snap.Router.MarkSuccess(candidate)
	}
}

// Models delegates to the current snapshot's router.
func (m *Manager) Models(ctx context.Context) ([]ir.Model, error) {
	snap := m.current.Load()
	if snap == nil || snap.Router == nil {
		return nil, fmt.Errorf("proxy runtime is not initialized")
	}
	return snap.Router.Models(ctx)
}

// buildProviders constructs provider adapters for every configured provider.
func buildProviders(cfg config.Config, httpClient *http.Client) (map[string]provider.Provider, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	providers := make(map[string]provider.Provider, len(cfg.Providers))
	for name, providerCfg := range cfg.Providers {
		tokenSource, err := auth.NewTokenSource(providerCfg.Auth)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
		p, err := provider.NewOpenAICompatible(name, providerCfg, tokenSource, httpClient)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
		providers[name] = p
	}
	return providers, nil
}

// reconcileAvailability keeps state only for provider/model pairs that still
// exist in the new config, so removed pairs are dropped and unchanged pairs
// preserve their exhaustion windows and failure counts.
func reconcileAvailability(states []router.AvailabilityState, cfg *config.Config) []router.AvailabilityState {
	if cfg == nil || len(states) == 0 {
		return nil
	}
	kept := make([]router.AvailabilityState, 0, len(states))
	for _, state := range states {
		providerCfg, ok := cfg.Providers[state.Provider]
		if !ok {
			continue
		}
		if _, ok := providerCfg.Models[state.Model]; !ok {
			continue
		}
		kept = append(kept, state)
	}
	return kept
}
