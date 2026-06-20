// Package auth provides upstream provider token sources.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"battle-proxy-akira/internal/config"
)

var (
	// ErrUnsupportedAuthType indicates that no token source exists for the configured auth type.
	ErrUnsupportedAuthType = errors.New("unsupported provider auth type")
	// ErrMissingToken indicates that a configured token source did not produce a token.
	ErrMissingToken = errors.New("provider token is not configured")
	// ErrMalformedToken indicates that a configured source returned malformed token data.
	ErrMalformedToken = errors.New("provider token data is malformed")
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

// AccessTokenEnvSource reads an OAuth/access-token style bearer token from an environment variable.
type AccessTokenEnvSource struct {
	env    string
	lookup func(string) (string, bool)
}

// AccessTokenFileSource reads an OAuth/access-token style bearer token from a local file.
type AccessTokenFileSource struct {
	file     string
	readFile func(string) ([]byte, error)
}

// AccessTokenCommandSource retrieves an OAuth/access-token style bearer token from a command.
type AccessTokenCommandSource struct {
	command []string
	run     func(context.Context, []string) ([]byte, error)
	now     func() time.Time
}

// NewBearerEnvTokenSource creates a token source for bearer_env provider auth.
func NewBearerEnvTokenSource(env string) (*BearerEnvTokenSource, error) {
	env = strings.TrimSpace(env)
	if env == "" {
		return nil, fmt.Errorf("bearer_env auth requires env name: %w", ErrMissingToken)
	}
	return &BearerEnvTokenSource{env: env, lookup: os.LookupEnv}, nil
}

// NewAccessTokenEnvSource creates a token source for env_access_token provider auth.
func NewAccessTokenEnvSource(env string) (*AccessTokenEnvSource, error) {
	env = strings.TrimSpace(env)
	if env == "" {
		return nil, fmt.Errorf("env_access_token auth requires env name: %w", ErrMissingToken)
	}
	return &AccessTokenEnvSource{env: env, lookup: os.LookupEnv}, nil
}

// NewAccessTokenFileSource creates a token source for file_access_token provider auth.
func NewAccessTokenFileSource(file string) (*AccessTokenFileSource, error) {
	file = strings.TrimSpace(file)
	if file == "" {
		return nil, fmt.Errorf("file_access_token auth requires file path: %w", ErrMissingToken)
	}
	return &AccessTokenFileSource{file: file, readFile: os.ReadFile}, nil
}

// NewAccessTokenCommandSource creates a token source for access_token_command provider auth.
func NewAccessTokenCommandSource(command []string) (*AccessTokenCommandSource, error) {
	clean := normalizeCommand(command)
	if len(clean) == 0 {
		return nil, fmt.Errorf("access_token_command auth requires command: %w", ErrMissingToken)
	}
	return &AccessTokenCommandSource{command: clean, run: runCommand, now: time.Now}, nil
}

// NewTokenSource creates a TokenSource for the supported provider auth config.
func NewTokenSource(auth config.AuthConfig) (TokenSource, error) {
	switch auth.Type {
	case config.AuthTypeBearerEnv:
		return NewBearerEnvTokenSource(auth.Env)
	case config.AuthTypeEnvAccessToken:
		return NewAccessTokenEnvSource(auth.Env)
	case config.AuthTypeFileAccessToken:
		return NewAccessTokenFileSource(auth.File)
	case config.AuthTypeCommandAccessToken:
		return NewAccessTokenCommandSource(auth.Command)
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
	return tokenFromEnv(s.env, s.lookup, "provider token")
}

// Token returns the current access token from an environment variable.
func (s *AccessTokenEnvSource) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil || s.env == "" {
		return "", fmt.Errorf("env_access_token token source is not configured: %w", ErrMissingToken)
	}
	return tokenFromEnv(s.env, s.lookup, "provider access token")
}

// Token returns the current access token from a local file.
func (s *AccessTokenFileSource) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil || s.file == "" {
		return "", fmt.Errorf("file_access_token token source is not configured: %w", ErrMissingToken)
	}
	data, err := s.readFile(s.file)
	if err != nil {
		return "", fmt.Errorf("read provider access token file %q: %w", s.file, ErrMissingToken)
	}
	return nonEmptyToken(string(data), "provider access token file")
}

// Token invokes the configured command and returns its access_token JSON field.
func (s *AccessTokenCommandSource) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil || len(s.command) == 0 {
		return "", fmt.Errorf("access_token_command token source is not configured: %w", ErrMissingToken)
	}
	out, err := s.run(ctx, s.command)
	if err != nil {
		return "", fmt.Errorf("provider access token command failed: %w", ErrMissingToken)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   string `json:"expires_at"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return "", fmt.Errorf("provider access token command output: %w", ErrMalformedToken)
	}
	if payload.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, payload.ExpiresAt)
		if err != nil {
			return "", fmt.Errorf("provider access token command expires_at: %w", ErrMalformedToken)
		}
		now := time.Now
		if s.now != nil {
			now = s.now
		}
		if !expiresAt.After(now()) {
			return "", fmt.Errorf("provider access token command returned expired token: %w", ErrMissingToken)
		}
	}
	return nonEmptyToken(payload.AccessToken, "provider access token command")
}

func tokenFromEnv(env string, lookup func(string) (string, bool), label string) (string, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	token, ok := lookup(env)
	if !ok {
		return "", fmt.Errorf("%s env %q is unset or empty: %w", label, env, ErrMissingToken)
	}
	return nonEmptyToken(token, label+" env "+env)
}

func nonEmptyToken(raw string, source string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", fmt.Errorf("%s is empty: %w", source, ErrMissingToken)
	}
	return token, nil
}

func normalizeCommand(command []string) []string {
	out := make([]string, 0, len(command))
	for _, arg := range command {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			out = append(out, arg)
		}
	}
	return out
}

func runCommand(ctx context.Context, command []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	return cmd.Output()
}
