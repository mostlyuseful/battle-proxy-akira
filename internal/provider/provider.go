// Package provider defines upstream model provider interfaces and adapters.
package provider

import (
	"context"
	"errors"

	"battle-proxy-akira/internal/ir"
)

var (
	// ErrStreamingUnsupported is returned by providers before streaming support is implemented.
	ErrStreamingUnsupported = errors.New("provider streaming is not implemented")
)

// Provider is the provider-neutral interface used by routing and API layers.
type Provider interface {
	Name() string
	Complete(ctx context.Context, req ir.Request) (*ir.Response, error)
	Stream(ctx context.Context, req ir.Request) (<-chan ir.Event, error)
	Models(ctx context.Context) ([]ir.Model, error)
	Health(ctx context.Context) error
}
