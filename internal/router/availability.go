package router

import (
	"errors"
	"sort"
	"sync"
	"time"

	providerpkg "battle-proxy-akira/internal/provider"
)

const (
	exhaustionBackoffBase = 30 * time.Second
	exhaustionBackoffMax  = 5 * time.Minute
)

// AvailabilityState is an in-memory snapshot for one provider/model pair.
type AvailabilityState struct {
	Provider       string
	Model          string
	Healthy        bool
	ExhaustedUntil *time.Time
	LastErrorCode  string
	Failures       int
}

type availabilityKey struct {
	provider string
	model    string
}

// AvailabilityTracker stores provider/model availability state safely across concurrent requests.
type AvailabilityTracker struct {
	mu     sync.RWMutex
	now    func() time.Time
	states map[availabilityKey]AvailabilityState
}

// NewAvailabilityTracker creates an empty in-memory availability tracker.
func NewAvailabilityTracker() *AvailabilityTracker {
	return &AvailabilityTracker{now: time.Now, states: map[availabilityKey]AvailabilityState{}}
}

// MarkSuccess records a successful candidate outcome and clears prior failure/exhaustion state.
func (t *AvailabilityTracker) MarkSuccess(candidate RouteCandidate) {
	if t == nil {
		return
	}
	key := availabilityKey{provider: candidate.ProviderName, model: candidate.ProviderModel}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.states[key] = AvailabilityState{
		Provider: candidate.ProviderName,
		Model:    candidate.ProviderModel,
		Healthy:  true,
	}
}

// MarkFailure records a failed candidate outcome.
func (t *AvailabilityTracker) MarkFailure(candidate RouteCandidate, err error) {
	if t == nil {
		return
	}
	key := availabilityKey{provider: candidate.ProviderName, model: candidate.ProviderModel}
	code := providerpkg.ErrorCode(err)
	if code == "" {
		code = providerpkg.ErrorUpstream
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.states[key]
	state.Provider = candidate.ProviderName
	state.Model = candidate.ProviderModel
	state.Healthy = false
	state.Failures++
	state.LastErrorCode = code
	state.ExhaustedUntil = t.exhaustedUntil(code, state.Failures, err)
	t.states[key] = state
}

// Get returns a snapshot of one provider/model state.
func (t *AvailabilityTracker) Get(providerName, modelName string) (AvailabilityState, bool) {
	if t == nil {
		return AvailabilityState{}, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	state, ok := t.states[availabilityKey{provider: providerName, model: modelName}]
	return cloneAvailabilityState(state), ok
}

// States returns all tracked state snapshots in deterministic order.
func (t *AvailabilityTracker) States() []AvailabilityState {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	states := make([]AvailabilityState, 0, len(t.states))
	for _, state := range t.states {
		states = append(states, cloneAvailabilityState(state))
	}
	sort.Slice(states, func(i, j int) bool {
		if states[i].Provider == states[j].Provider {
			return states[i].Model < states[j].Model
		}
		return states[i].Provider < states[j].Provider
	})
	return states
}

func (t *AvailabilityTracker) exhaustedUntil(code string, failures int, err error) *time.Time {
	if code != providerpkg.ErrorProviderRateLimited && code != providerpkg.ErrorProviderExhausted {
		return nil
	}
	var providerErr *providerpkg.Error
	if errors.As(err, &providerErr) && providerErr.RetryAfter != nil {
		until := *providerErr.RetryAfter
		return &until
	}
	now := time.Now
	if t.now != nil {
		now = t.now
	}
	backoff := exponentialBackoff(failures)
	until := now().Add(backoff)
	return &until
}

func exponentialBackoff(failures int) time.Duration {
	if failures <= 1 {
		return exhaustionBackoffBase
	}
	backoff := exhaustionBackoffBase
	for i := 1; i < failures; i++ {
		if backoff >= exhaustionBackoffMax/2 {
			return exhaustionBackoffMax
		}
		backoff *= 2
	}
	if backoff > exhaustionBackoffMax {
		return exhaustionBackoffMax
	}
	return backoff
}

// IsAvailable reports whether a provider/model is not in an active exhaustion window.
func (t *AvailabilityTracker) IsAvailable(providerName, modelName string, at time.Time) bool {
	if t == nil {
		return true
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	state, ok := t.states[availabilityKey{provider: providerName, model: modelName}]
	if !ok || state.ExhaustedUntil == nil {
		return true
	}
	return !state.ExhaustedUntil.After(at)
}

func cloneAvailabilityState(state AvailabilityState) AvailabilityState {
	if state.ExhaustedUntil != nil {
		until := *state.ExhaustedUntil
		state.ExhaustedUntil = &until
	}
	return state
}
