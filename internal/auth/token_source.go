// Package auth provides upstream provider token sources.
package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"battle-proxy-akira/internal/config"
)

var (
	// ErrUnsupportedAuthType indicates that no token source exists for the configured auth type.
	ErrUnsupportedAuthType = errors.New("unsupported provider auth type")
	// ErrMissingToken indicates that a configured token source did not produce a token.
	ErrMissingToken = errors.New("provider token is not configured")
)

// TokenSource retrieves an upstream provider bearer token.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// BearerEnvTokenSource reads a provider API key from an environment variable.
type BearerEnvTokenSource struct {
	env    string
	lookup func(string) (string, bool)
}

// NewBearerEnvTokenSource creates a token source for bearer_env provider auth.
func NewBearerEnvTokenSource(env string) (*BearerEnvTokenSource, error) {
	env = strings.TrimSpace(env)
	if env == "" {
		return nil, fmt.Errorf("bearer_env auth requires env name: %w", ErrMissingToken)
	}
	return &BearerEnvTokenSource{env: env, lookup: os.LookupEnv}, nil
}

// NewTokenSource creates a TokenSource for the supported provider auth config.
func NewTokenSource(auth config.AuthConfig) (TokenSource, error) {
	switch auth.Type {
	case config.AuthTypeBearerEnv:
		return NewBearerEnvTokenSource(auth.Env)
	default:
		return nil, fmt.Errorf("%s: %w", auth.Type, ErrUnsupportedAuthType)
	}
}

// Token returns the current bearer token, or an error that identifies only the env var name.
func (s *BearerEnvTokenSource) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil || s.env == "" {
		return "", fmt.Errorf("bearer_env token source is not configured: %w", ErrMissingToken)
	}

	token, ok := s.lookup(s.env)
	if !ok || strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("provider token env %q is unset or empty: %w", s.env, ErrMissingToken)
	}
	return token, nil
}
